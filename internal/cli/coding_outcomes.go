package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func outcomes(args []string, out, errOut io.Writer, base string) int {
	method := "GET"
	var body []byte
	var project string
	if len(args) == 1 {
		project = args[0]
	} else if len(args) == 3 && args[0] == "record" {
		project = args[1]
		var err error
		body, err = os.ReadFile(args[2])
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		if len(body) > 64<<10 {
			fmt.Fprintln(errOut, "outcome file exceeds 64 KiB")
			return 2
		}
		method = "POST"
	} else {
		fmt.Fprintln(errOut, "usage: tk outcomes <project> | tk outcomes record <project> <review.json>")
		return 2
	}
	id, err := resolveProjectID(base, project)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+"/api/v1/projects/"+id+"/coding-outcomes", bytes.NewReader(body))
	if err != nil {
		return 1
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := newHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(errOut, resp.Body)
		return 1
	}
	io.Copy(out, resp.Body)
	return 0
}
