package main

import (
	"encoding/json"
	"fmt"
	"github.com/bitfield/script"
	"k8s.io/apimachinery/pkg/util/wait"
	"strings"
	"time"
)

type cluster struct {
	cli string
}

func (c *cluster) ApplyYaml(content string) error {
	return exec(fmt.Sprintf("%s apply -f -", c.cli), content)
}

func (c *cluster) ApplyYamlToNamespace(namespace, content string) error {
	return exec(fmt.Sprintf("%s apply -f - -n %s", c.cli, namespace), content)
}

func (c *cluster) GetAndFilterResourceByJq(resType string, name string, namespace string, jqExpr string) (string, error) {
	cmd := fmt.Sprintf("%s get %s %s -n %s -o json", c.cli, resType, name, namespace)
	out := &strings.Builder{}
	p := script.NewPipe().WithStdout(out).Exec(cmd).JQ(jqExpr)
	p.Stdout()
	if err := p.Error(); err != nil {
		return "", err
	}
	p.Wait()
	return out.String(), nil
}

func (c *cluster) WaitForResourceValueMatch(resType string, name string, namespace string, jqExpr string, value string) error {
	return wait.PollImmediate(5*time.Second, 60*time.Second, func() (done bool, err error) {
		rawJson, err := c.GetAndFilterResourceByJq(resType, name, namespace, jqExpr)
		if err != nil {
			return false, nil
		}

		var val string
		if err = json.Unmarshal([]byte(rawJson), &val); err != nil {
			return false, nil
		}

		return val == value, nil
	})
}

func (c *cluster) WaitForResourceConditionMatch(resType string, name string, namespace string, conditions map[string]string) error {
	for k, v := range conditions {
		if err := c.WaitForResourceValueMatch(resType, name, namespace, fmt.Sprintf(".status.conditions[] | select(.type==\"%s\").status", k), v); err != nil {
			return err
		}
	}
	return nil
}

func exec(cmd string, stdin string) error {
	out := &strings.Builder{}
	p := script.Echo(stdin).WithStdout(out).Exec(cmd)
	p.Stdout()
	p.Wait()
	if p.ExitStatus() != 0 {
		return fmt.Errorf("error code returned %d stdout %s", p.ExitStatus(), out.String())
	}
	return nil
}

func (c *cluster) createNamespace(namespace string) error {
	out := &strings.Builder{}
	p := script.NewPipe().WithStdout(out).Exec(fmt.Sprintf("%s create ns %s --dry-run=client -o yaml", c.cli, namespace)).Exec(c.cli + " apply -f -")
	p.Stdout()
	if err := p.Error(); err != nil {
		return err
	}
	p.Wait()
	if p.ExitStatus() != 0 {
		return fmt.Errorf("error code returned %d \n%s", p.ExitStatus(), out.String())
	}
	return nil
}
