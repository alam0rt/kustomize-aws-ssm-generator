package main

import (
	"testing"

	"github.com/ghodss/yaml"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type metadata struct {
	Name string `yaml:"name"`
}

var testConfig Config

func init() {
	testConfig = Config{
		APIVersion: "k8s.samlockart.com/v1",
		Kind:       "ParameterStore",
		Metadata: metadata{
			Name: "test",
		},
		Path:     "/",
		Version:  1,
		Annotate: true,
	}
}

func TestSecretObject(t *testing.T) {
	name := "test"
	var data secretData
	s := &secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: v1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: testConfig.Metadata.Name,
		},
		Type: v1.SecretTypeOpaque,
		Data: data,
	}

	if s.Name != name {
		t.Errorf("Secret doesn't have correct name, got: %s, want: %s", s.Name, name)
	}
}

func TestSecretData(t *testing.T) {
	value := "blah blah"
	d := secretData{
		string("test"): []byte(value),
	}
	s := &secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: v1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: testConfig.Metadata.Name,
		},
		Type: v1.SecretTypeOpaque,
		Data: d,
	}

	m := s.Marshal()
	x := &secret{}

	yaml.Unmarshal(m, x)

	if string(x.Data["test"]) != value {
		t.Errorf("There was a problem unmarshalling the secret. Expected secret value of: %s, got: %s", string(x.Data["test"]), value)
	}

}
