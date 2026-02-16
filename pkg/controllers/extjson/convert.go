package extjson

import (
	"bytes"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func IsEmpty(j *apiextv1.JSON) bool {
	return j == nil || len(j.Raw) == 0 || bytes.Equal(j.Raw, []byte(`null`))
}

// TryToYAML attempts to convert [apiextv1.JSON] to a YAML string.
// It returns an empty string if the input is nil, contains no data, or if the conversion from JSON to YAML fails.
func TryToYAML(j *apiextv1.JSON) string {
	if IsEmpty(j) {
		return ""
	}
	b, err := yaml.JSONToYAML(j.Raw)
	if err != nil {
		return ""
	}
	return string(b)
}

// TryFromYAML attempts to convert a YAML string to [apiextv1.JSON].
// It returns nil if the input is empty or if the conversion fails, effectively swallowing any parsing errors.
func TryFromYAML(s string) *apiextv1.JSON {
	if len(s) == 0 {
		return nil
	}
	b, err := yaml.YAMLToJSON([]byte(s))
	if err != nil {
		return nil
	}
	return &apiextv1.JSON{Raw: b}
}
