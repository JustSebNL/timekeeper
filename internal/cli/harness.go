package cli

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	harnesscatalog "github.com/JustSebNL/timekeeper/internal/harness"
)

type harnessItem = harnesscatalog.Item

func harnessCommand(args []string, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: tk harness <list|show> [item]")
		fmt.Fprintln(out, "  list                  List bundled harness documents and Skills")
		fmt.Fprintln(out, "  show <listed-item>    Print the exact contents of one listed item")
		return 0
	}
	root, err := findHarnessRoot()
	if err != nil {
		fmt.Fprintf(errOut, "harness: %v\n", err)
		return 1
	}
	items, err := harnessEntries(root)
	if err != nil {
		fmt.Fprintf(errOut, "harness: %v\n", err)
		return 1
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: tk harness list")
			return 2
		}
		fmt.Fprintln(out, "ID\tPATH\tDESCRIPTION\tURL")
		for _, item := range items {
			link := "http://timekeeper.local/harness.html?item=" + url.QueryEscape(item.ID)
			fmt.Fprintf(out, "%s	%s	%s	%s\n", item.ID, item.Path, item.Description, link)
		}
		return 0
	case "show":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			fmt.Fprintln(errOut, "usage: tk harness show <listed-item>")
			return 2
		}
		return showHarnessItem(root, items, args[1], out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown harness command %q (use list|show)\n", args[0])
		return 2
	}
}

func findHarnessRoot() (string, error) {
	return harnesscatalog.FindRoot()
}

func harnessEntries(root string) ([]harnessItem, error) {
	return harnesscatalog.Entries(root)
}

func showHarnessItem(root string, _ []harnessItem, identifier string, out, errOut io.Writer) int {
	item, content, err := harnesscatalog.Lookup(root, identifier)
	if err != nil {
		fmt.Fprintf(errOut, "harness: %v; run `tk harness list`\n", err)
		return 2
	}
	if _, err := out.Write(content); err != nil {
		fmt.Fprintf(errOut, "write harness item %s: %v\n", item.ID, err)
		return 1
	}
	return 0
}
