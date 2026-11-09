package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSuite(t *testing.T) {
	suite, err := LoadSuite(filepath.Join("..", "..", "testdata", "integration"))
	if err != nil {
		t.Fatal(err)
	}
	if len(suite.Tests) != 3 {
		t.Fatalf("tests = %d", len(suite.Tests))
	}
	if suite.Tests[0].Definition.Name == "" || len(suite.Tests[0].Definition.Steps) == 0 {
		t.Fatalf("first test = %+v", suite.Tests[0])
	}
}

func TestLoadSuiteRejectsInvalidJTD(t *testing.T) {
	dir := t.TempDir()
	schema, err := os.ReadFile(filepath.Join("..", "..", "testdata", "integration", "schema.jtd.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.jtd.json"), schema, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"name": 42, "steps": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSuite(dir); err == nil {
		t.Fatal("expected JTD validation error")
	}
}

func TestLoadSuiteRejectsSemanticErrors(t *testing.T) {
	dir := t.TempDir()
	schema, err := os.ReadFile(filepath.Join("..", "..", "testdata", "integration", "schema.jtd.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.jtd.json"), schema, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"name":"bad","steps":[{"type":"http"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSuite(dir); err == nil || !strings.Contains(err.Error(), "method and path") {
		t.Fatalf("error = %v", err)
	}
}

func TestExpandVariables(t *testing.T) {
	vars := map[string]string{"run_id": "abc", "base_url": "http://x"}
	got, err := expandString("/api/${run_id}/${base_url}", vars)
	if err != nil || got != "/api/abc/http://x" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := expandString("${missing}", vars); err == nil {
		t.Fatal("expected missing variable error")
	}
	value := map[string]any{"a": []any{"${run_id}", 3}, "b": "${base_url}"}
	expanded, err := expandValue(value, vars)
	if err != nil {
		t.Fatal(err)
	}
	if expanded.(map[string]any)["b"] != "http://x" || expanded.(map[string]any)["a"].([]any)[0] != "abc" {
		t.Fatalf("expanded = %#v", expanded)
	}
}

func TestLookupAndAssertions(t *testing.T) {
	data := map[string]any{
		"count": float64(2),
		"items": []any{
			map[string]any{"id": "a", "subject": "Hello"},
			map[string]any{"id": "b", "subject": "World"},
		},
	}
	if value, ok := lookupPath(data, "items.1.id"); !ok || value != "b" {
		t.Fatalf("lookup = %#v %v", value, ok)
	}
	if _, ok := lookupPath(data, "items.9"); ok {
		t.Fatal("expected missing index")
	}
	equals := any(float64(2))
	exists := true
	length := 2
	expect := &Expectation{JSON: []JSONAssertion{
		{Path: "count", Equals: &equals},
		{Path: "items", Length: &length},
		{Path: "items.0.subject", Contains: "Hell"},
		{Path: "items.1.id", Exists: &exists},
	}}
	if err := checkExpectation(expect, 200, nil, nil, data); err != nil {
		t.Fatal(err)
	}
	bad := any(float64(3))
	expect.JSON[0].Equals = &bad
	if err := checkExpectation(expect, 200, nil, nil, data); err == nil {
		t.Fatal("expected equals failure")
	}
}

func TestSaveValues(t *testing.T) {
	data := map[string]any{"token": "secret", "nested": map[string]any{"id": "x"}, "n": float64(3)}
	values, err := saveValues(map[string]string{"api_key": "token", "id": "nested.id", "number": "n"}, data)
	if err != nil {
		t.Fatal(err)
	}
	if values["api_key"] != "secret" || values["id"] != "x" || values["number"] != "3" {
		t.Fatalf("values = %#v", values)
	}
	if _, err := saveValues(map[string]string{"missing": "nope"}, data); err == nil {
		t.Fatal("expected missing path error")
	}
}

func TestBuildMessage(t *testing.T) {
	message, err := buildMessage(Step{
		From: "sender@example.org", To: []string{"inbox@example.test"},
		Subject: "Hello", Text: "plain", HTML: "<p>html</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(message)
	for _, want := range []string{"multipart/alternative", "Content-Type: text/plain", "Content-Type: text/html", "plain", "<p>html</p>"} {
		if !strings.Contains(text, want) {
			t.Errorf("message missing %q: %s", want, text)
		}
	}
	if _, err := buildMessage(Step{From: "a@b\r\nX", To: []string{"c@d"}, Subject: "x"}); err == nil {
		t.Fatal("expected newline rejection")
	}
}

func TestFilterTests(t *testing.T) {
	tests := []SuiteTest{
		{File: "a.json", Definition: TestDefinition{Name: "Alpha"}},
		{File: "b.json", Definition: TestDefinition{Name: "Beta"}},
	}
	if got := filterTests(tests, "alpha"); len(got) != 1 || got[0].Definition.Name != "Alpha" {
		t.Fatalf("alpha = %+v", got)
	}
	if got := filterTests(tests, "BETA,b.json"); len(got) != 1 {
		t.Fatalf("beta = %+v", got)
	}
	if got := filterTests(tests, ""); len(got) != 2 {
		t.Fatalf("all = %+v", got)
	}
}
