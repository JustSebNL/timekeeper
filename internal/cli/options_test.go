// Copyright (c) 2026 Seb. All rights reserved.

package cli

import (
	"reflect"
	"testing"
)

func TestResolveBaseURLPrefersExplicitURLOverEnvironment(t *testing.T) {
	baseURL, commandArgs, err := ResolveBaseURL([]string{"--url", "http://127.0.0.1:18095", "doctor"}, "http://127.0.0.1:1618")
	if err != nil {
		t.Fatal(err)
	}
	if baseURL != "http://127.0.0.1:18095" {
		t.Fatalf("baseURL=%q", baseURL)
	}
	if want := []string{"doctor"}; !reflect.DeepEqual(commandArgs, want) {
		t.Fatalf("commandArgs=%#v want=%#v", commandArgs, want)
	}
}

func TestResolveBaseURLRejectsMissingExplicitValue(t *testing.T) {
	_, _, err := ResolveBaseURL([]string{"--url"}, "")
	if err == nil {
		t.Fatal("expected missing --url value error")
	}
}
