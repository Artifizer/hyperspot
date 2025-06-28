package utils

import (
	"bytes"
	"fmt"
	"math/rand"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

func seq(n int) []int {
	seq := make([]int, n)
	for i := 0; i < n; i++ {
		seq[i] = i
	}
	return seq
}

// randInt generates a random integer between min and max (inclusive)
func randInt(min, max int) int {
	return rand.Intn(max-min+1) + min
}

var tmplFuncs = sprig.TxtFuncMap()

func init() {
	tmplFuncs["randInt"] = randInt
	tmplFuncs["seq"] = seq
}

func TemplateToText(tmpl string) (string, error) {
	t, err := template.New("template").Funcs(tmplFuncs).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var out bytes.Buffer
	if err := t.Execute(&out, nil); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return out.String(), nil
}
