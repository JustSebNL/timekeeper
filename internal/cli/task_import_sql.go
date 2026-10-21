package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/model"
)

type sqlBatchValue struct {
	text   string
	quoted bool
	null   bool
}

func parseStrictTaskSQL(data []byte) ([]model.CreateTaskInput, error) {
	statements, err := splitTaskSQLStatements(string(data))
	if err != nil {
		return nil, err
	}
	var tasks []model.CreateTaskInput
	for statementIndex, statement := range statements {
		columns, values, err := parseTaskInsertStatement(statement)
		if err != nil {
			return nil, fmt.Errorf("statement %d: %w", statementIndex+1, err)
		}
		header, err := taskImportHeaderMap(columns)
		if err != nil {
			return nil, fmt.Errorf("statement %d columns: %w", statementIndex+1, err)
		}
		rows, err := parseTaskSQLValueTuples(values, columns)
		if err != nil {
			return nil, fmt.Errorf("statement %d values: %w", statementIndex+1, err)
		}
		for rowIndex, row := range rows {
			task, err := processRow(header, row)
			if err != nil {
				return nil, fmt.Errorf("statement %d row %d: %w", statementIndex+1, rowIndex+1, err)
			}
			tasks = append(tasks, task)
		}
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("no task rows found in SQL file")
	}
	return tasks, nil
}

func splitTaskSQLStatements(text string) ([]string, error) {
	var statements []string
	var current strings.Builder
	inQuote, inLineComment, inBlockComment := false, false, false
	for i := 0; i < len(text); i++ {
		c := text[i]
		var next byte
		if i+1 < len(text) {
			next = text[i+1]
		}
		if inLineComment {
			if c == '\n' {
				inLineComment = false
				current.WriteByte('\n')
			}
			continue
		}
		if inBlockComment {
			if c == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inQuote {
			current.WriteByte(c)
			if c == '\'' {
				if next == '\'' {
					current.WriteByte(next)
					i++
				} else {
					inQuote = false
				}
			}
			continue
		}
		if c == '-' && next == '-' {
			inLineComment = true
			i++
			continue
		}
		if c == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		if c == '\'' {
			inQuote = true
			current.WriteByte(c)
			continue
		}
		if c == ';' {
			if stmt := strings.TrimSpace(current.String()); stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	if inQuote || inBlockComment {
		return nil, fmt.Errorf("unterminated SQL string or block comment")
	}
	if strings.TrimSpace(current.String()) != "" {
		return nil, fmt.Errorf("SQL statements must end with a semicolon")
	}
	return statements, nil
}

func parseTaskInsertStatement(statement string) ([]string, string, error) {
	const prefix = "INSERT INTO"
	statement = strings.TrimSpace(statement)
	if !strings.HasPrefix(strings.ToUpper(statement), prefix) {
		return nil, "", fmt.Errorf("only INSERT INTO tasks (...) VALUES (...) statements are accepted")
	}
	rest := strings.TrimSpace(statement[len(prefix):])
	open := strings.IndexByte(rest, '(')
	if open < 0 || !strings.EqualFold(strings.TrimSpace(rest[:open]), "tasks") {
		return nil, "", fmt.Errorf("target table must be tasks and include a column list")
	}
	closeOffset := strings.IndexByte(rest[open+1:], ')')
	if closeOffset < 0 {
		return nil, "", fmt.Errorf("unterminated task column list")
	}
	close := open + 1 + closeOffset
	columns := strings.Split(rest[open+1:close], ",")
	for i := range columns {
		columns[i] = strings.TrimSpace(columns[i])
	}
	after := strings.TrimSpace(rest[close+1:])
	if len(after) < len("VALUES") || !strings.EqualFold(after[:len("VALUES")], "VALUES") {
		return nil, "", fmt.Errorf("task INSERT must use VALUES")
	}
	values := strings.TrimSpace(after[len("VALUES"):])
	if values == "" {
		return nil, "", fmt.Errorf("VALUES list is empty")
	}
	return columns, values, nil
}

func parseTaskSQLValueTuples(input string, columns []string) ([][]string, error) {
	var rows [][]string
	index := 0
	for {
		index = skipTaskSQLSpace(input, index)
		if index == len(input) {
			break
		}
		if input[index] != '(' {
			return nil, fmt.Errorf("expected '(' before row %d", len(rows)+1)
		}
		index++
		row := make([]string, 0, len(columns))
		for columnIndex, column := range columns {
			index = skipTaskSQLSpace(input, index)
			value, next, err := parseTaskSQLLiteral(input, index)
			if err != nil {
				return nil, fmt.Errorf("row %d column %s: %w", len(rows)+1, column, err)
			}
			text, err := validateTaskSQLLiteral(column, value)
			if err != nil {
				return nil, fmt.Errorf("row %d: %w", len(rows)+1, err)
			}
			row = append(row, text)
			index = skipTaskSQLSpace(input, next)
			if columnIndex < len(columns)-1 {
				if index >= len(input) || input[index] != ',' {
					return nil, fmt.Errorf("row %d has fewer values than columns", len(rows)+1)
				}
				index++
			} else if index >= len(input) || input[index] != ')' {
				return nil, fmt.Errorf("row %d has more values than columns", len(rows)+1)
			}
		}
		index++
		rows = append(rows, row)
		index = skipTaskSQLSpace(input, index)
		if index == len(input) {
			break
		}
		if input[index] != ',' {
			return nil, fmt.Errorf("expected ',' between VALUES rows")
		}
		index++
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("VALUES contains no rows")
	}
	return rows, nil
}

func parseTaskSQLLiteral(input string, index int) (sqlBatchValue, int, error) {
	if index >= len(input) {
		return sqlBatchValue{}, index, fmt.Errorf("missing value")
	}
	if input[index] == '\'' {
		var value strings.Builder
		for i := index + 1; i < len(input); i++ {
			if input[i] != '\'' {
				value.WriteByte(input[i])
				continue
			}
			if i+1 < len(input) && input[i+1] == '\'' {
				value.WriteByte('\'')
				i++
				continue
			}
			return sqlBatchValue{text: value.String(), quoted: true}, i + 1, nil
		}
		return sqlBatchValue{}, index, fmt.Errorf("unterminated quoted string")
	}
	start := index
	for index < len(input) && input[index] != ',' && input[index] != ')' {
		index++
	}
	token := strings.TrimSpace(input[start:index])
	if token == "" {
		return sqlBatchValue{}, index, fmt.Errorf("empty unquoted value")
	}
	if strings.EqualFold(token, "NULL") {
		return sqlBatchValue{null: true}, index, nil
	}
	if strings.EqualFold(token, "TRUE") || strings.EqualFold(token, "FALSE") {
		return sqlBatchValue{text: strings.ToLower(token)}, index, nil
	}
	if _, err := strconv.ParseFloat(token, 64); err != nil {
		return sqlBatchValue{}, index, fmt.Errorf("unquoted value %q is not numeric, boolean, or NULL", token)
	}
	return sqlBatchValue{text: token}, index, nil
}

func validateTaskSQLLiteral(column string, value sqlBatchValue) (string, error) {
	if value.null {
		return "", nil
	}
	switch column {
	case "estimated_duration_seconds":
		if value.quoted {
			return "", fmt.Errorf("estimated_duration_seconds must be an unquoted non-negative integer")
		}
		seconds, err := strconv.ParseInt(value.text, 10, 64)
		if err != nil || seconds < 0 {
			return "", fmt.Errorf("estimated_duration_seconds must be an unquoted non-negative integer")
		}
	case "completion_pct", "reported_completion_pct", "calculated_completion_pct":
		if value.quoted {
			return "", fmt.Errorf("%s must be an unquoted finite number", column)
		}
		number, err := strconv.ParseFloat(value.text, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return "", fmt.Errorf("%s must be an unquoted finite number", column)
		}
	case "auto_close":
		if _, err := parseAutoCloseStrict(value.text); err != nil {
			return "", err
		}
	default:
		if !value.quoted {
			return "", fmt.Errorf("%s must be a single-quoted string", column)
		}
	}
	return value.text, nil
}

func skipTaskSQLSpace(input string, index int) int {
	for index < len(input) && (input[index] == ' ' || input[index] == '\t' || input[index] == '\r' || input[index] == '\n') {
		index++
	}
	return index
}
