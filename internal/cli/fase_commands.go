package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func faseCommand(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 1 {
		return 2
	}
	method, path := http.MethodGet, ""
	switch args[0] {
	case "profile":
		id := resolveAgentIdentity().AgentID
		if len(args) == 2 {
			id = args[1]
		} else if len(args) != 1 {
			return 2
		}
		if id == "" {
			fmt.Fprintln(errOut, "profile requires a resolved FASE identity")
			return 2
		}
		path = "/agent-profiles/" + url.PathEscape(id)
	case "claims":
		if len(args) != 2 {
			fmt.Fprintln(errOut, "usage: tk agent claims <project-id>")
			return 2
		}
		path = "/projects/" + url.PathEscape(args[1]) + "/execution-claims"
	case "claim", "release":
		if len(args) != 2 {
			fmt.Fprintln(errOut, "usage: tk agent "+args[0]+" <task-or-sprint-id>")
			return 2
		}
		method = http.MethodPost
		path = "/execution-claims/" + url.PathEscape(args[1])
		if args[0] == "release" {
			path += "/release"
		}
	default:
		return 2
	}
	r, e := http.NewRequest(method, strings.TrimRight(baseURL, "/")+"/api/v1"+path, strings.NewReader("{}"))
	if e != nil {
		fmt.Fprintln(errOut, e)
		return 1
	}
	r.Header.Set("Content-Type", "application/json")
	response, e := newHTTPClient(10 * time.Second).Do(r)
	if e != nil {
		fmt.Fprintln(errOut, e)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		reportHTTPStatus(errOut, "agent "+args[0], response)
		return 1
	}
	if _, e = io.Copy(out, response.Body); e != nil {
		fmt.Fprintln(errOut, e)
		return 1
	}
	return 0
}
