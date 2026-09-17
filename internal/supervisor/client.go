package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/JustSebNL/timekeeper/internal/model"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	Base string
	HTTP *http.Client
}

func (c Client) call(ctx context.Context, method, path string, in, out any) error {
	var b []byte
	var err error
	if in != nil {
		b, err = json.Marshal(in)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Base, "/")+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("TimeKeeper %s: %s", resp.Status, body)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
func (c Client) find(ctx context.Context, p, id string) (model.Sprint, error) {
	var tree model.ExecutionTree
	if err := c.call(ctx, "GET", "/api/v1/projects/"+url.PathEscape(p)+"/execution-tree", nil, &tree); err != nil {
		return model.Sprint{}, err
	}
	var walk func([]model.ExecutionCategory) (model.Sprint, bool)
	walk = func(cs []model.ExecutionCategory) (model.Sprint, bool) {
		for _, cat := range cs {
			for _, t := range cat.Tasks {
				for _, s := range t.Sprints {
					if s.SprintID == id {
						return s, true
					}
				}
				for _, st := range t.Subtasks {
					for _, s := range st.Sprints {
						if s.SprintID == id {
							return s, true
						}
					}
				}
			}
			if s, ok := walk(cat.Categories); ok {
				return s, true
			}
		}
		return model.Sprint{}, false
	}
	if s, ok := walk(tree.Categories); ok {
		return s, nil
	}
	return model.Sprint{}, fmt.Errorf("Sprint %s is not in project %s", id, p)
}
