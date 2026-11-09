package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	jtd "github.com/jsontypedef/json-typedef-go"
)

// TestDefinition is one JTD-validated integration test.
type TestDefinition struct {
	Schema      string `json:"$schema,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Steps       []Step `json:"steps"`
}

// Step is one integration action.
type Step struct {
	Type      string            `json:"type"`
	Name      string            `json:"name,omitempty"`
	Method    string            `json:"method,omitempty"`
	Path      string            `json:"path,omitempty"`
	Auth      string            `json:"auth,omitempty"`
	Bearer    string            `json:"bearer,omitempty"`
	Basic     *BasicAuth        `json:"basic,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      any               `json:"body,omitempty"`
	BodyRaw   string            `json:"body_raw,omitempty"`
	Depth     string            `json:"depth,omitempty"`
	Tool      string            `json:"tool,omitempty"`
	Arguments map[string]any    `json:"arguments,omitempty"`
	From      string            `json:"from,omitempty"`
	To        []string          `json:"to,omitempty"`
	Subject   string            `json:"subject,omitempty"`
	Text      string            `json:"text,omitempty"`
	HTML      string            `json:"html,omitempty"`
	Port      string            `json:"port,omitempty"`
	Millis    int               `json:"millis,omitempty"`
	Save      map[string]string `json:"save,omitempty"`
	Expect    *Expectation      `json:"expect,omitempty"`
}

// BasicAuth is a WebDAV credential.
type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Expectation defines assertions for a step.
type Expectation struct {
	Status          int               `json:"status,omitempty"`
	BodyContains    []string          `json:"body_contains,omitempty"`
	BodyNotContains []string          `json:"body_not_contains,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	JSON            []JSONAssertion   `json:"json,omitempty"`
}

// JSONAssertion checks one JSON path in an HTTP or MCP result.
type JSONAssertion struct {
	Path     string   `json:"path"`
	Equals   *any     `json:"equals,omitempty"`
	Contains string   `json:"contains,omitempty"`
	Exists   *bool    `json:"exists,omitempty"`
	GTE      *float64 `json:"gte,omitempty"`
	LTE      *float64 `json:"lte,omitempty"`
	Length   *int     `json:"length,omitempty"`
}

// Suite is the set of loaded tests.
type Suite struct {
	Dir   string
	Tests []SuiteTest
}

// SuiteTest pairs a file with its definition.
type SuiteTest struct {
	File       string
	Definition TestDefinition
}

// LoadSuite loads and validates every test JSON file against the JTD schema.
func LoadSuite(dir string) (Suite, error) {
	schemaPath := filepath.Join(dir, "schema.jtd.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return Suite{}, fmt.Errorf("read schema: %w", err)
	}
	var schema jtd.Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return Suite{}, fmt.Errorf("parse schema: %w", err)
	}
	if err := schema.Validate(); err != nil {
		return Suite{}, fmt.Errorf("invalid JTD schema: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return Suite{}, err
	}
	sort.Strings(files)
	suite := Suite{Dir: dir}
	for _, file := range files {
		if filepath.Base(file) == "schema.jtd.json" {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return Suite{}, fmt.Errorf("%s: %w", file, err)
		}
		var instance any
		if err := json.Unmarshal(raw, &instance); err != nil {
			return Suite{}, fmt.Errorf("%s: invalid JSON: %w", file, err)
		}
		validationErrors, err := jtd.Validate(schema, instance)
		if err != nil {
			return Suite{}, fmt.Errorf("%s: JTD validation failed: %w", file, err)
		}
		if len(validationErrors) > 0 {
			return Suite{}, fmt.Errorf("%s: JTD validation: %s", file, formatJTDUnexpectedErrors(validationErrors))
		}
		var def TestDefinition
		if err := json.Unmarshal(raw, &def); err != nil {
			return Suite{}, fmt.Errorf("%s: %w", file, err)
		}
		if err := validateDefinition(def); err != nil {
			return Suite{}, fmt.Errorf("%s: %w", file, err)
		}
		suite.Tests = append(suite.Tests, SuiteTest{File: file, Definition: def})
	}
	if len(suite.Tests) == 0 {
		return Suite{}, fmt.Errorf("no test JSON files found in %s", dir)
	}
	return suite, nil
}

func validateDefinition(def TestDefinition) error {
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("test name is required")
	}
	if len(def.Steps) == 0 {
		return fmt.Errorf("test must contain at least one step")
	}
	for i, step := range def.Steps {
		prefix := fmt.Sprintf("step %d (%s)", i+1, step.Type)
		switch step.Type {
		case "http", "webdav":
			if step.Method == "" || step.Path == "" {
				return fmt.Errorf("%s: method and path are required", prefix)
			}
		case "mcp":
			if step.Tool == "" {
				return fmt.Errorf("%s: tool is required", prefix)
			}
		case "smtp":
			if step.From == "" || len(step.To) == 0 {
				return fmt.Errorf("%s: from and to are required", prefix)
			}
		case "sleep":
			if step.Millis <= 0 {
				return fmt.Errorf("%s: millis must be greater than zero", prefix)
			}
		default:
			return fmt.Errorf("%s: unknown step type", prefix)
		}
	}
	return nil
}

func formatJTDUnexpectedErrors(errs []jtd.ValidateError) string {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, fmt.Sprintf("%s: %s", err.InstancePath, err.SchemaPath))
	}
	return strings.Join(parts, "; ")
}
