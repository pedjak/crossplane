package main

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"strings"
)

const scenarioContextKey = "scenarioContext"

type resourceRef struct {
	Kind       string
	ApiVersion string
	Name       string
}

type scenarioContext struct {
	Cluster                   *cluster
	Namespace                 string
	Claim                     *unstructured.Unstructured
	ClaimCompositeResourceRef *resourceRef
	Configuration             *unstructured.Unstructured
}

func ScenarioContextValue(ctx context.Context) *scenarioContext {
	return ctx.Value(scenarioContextKey).(*scenarioContext)
}

func ToUnstructured(yamlContent string) (*unstructured.Unstructured, error) {
	m := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(yamlContent), &m)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: m}, nil
}

func (ref *resourceRef) Type() string {
	return fmt.Sprintf("%s.%s", strings.ToLower(ref.Kind), strings.Split(ref.ApiVersion, "/")[0])
}
