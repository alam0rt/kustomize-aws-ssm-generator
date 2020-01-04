package main

import (
	"fmt"
	"os"

	"encoding/json"

	"github.com/ghodss/yaml"

	"io/ioutil"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Config is a structure which holds the config map
// data. The config is passed to the application as a path
// and is then unmarshalled into this struct.
type Config struct {
	Version  string `yaml:"apiVersion"`
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	}
	Path string `yaml:"path"`
}

// secretData is a map holding the secret resource data.
// The reason it being a type is so we can add methods to it easily.
type secretData map[string][]byte

// Ditto
type secret v1.Secret

func (s *secret) Print() {
	fmt.Printf("%s\n", string(s.Marshal()))
}

func (s *secret) String() string {
	return string(s.Marshal())
}

// Marshal() simply marshals the secret resorce into a JSON byte stream
// and then we call JSONToYAML to convert that. The reason being, marshalling
// from struct to YAML does NOT honor the *v1.Secret struct's `json:",omitempty" directives.
// If we don't honor those, we get a large YAML resource with lots of empty fields.
func (s *secret) Marshal() []byte {
	j, err := json.Marshal(s)
	if err != nil {
		fmt.Print(err)
	}

	y, err := yaml.JSONToYAML(j)
	if err != nil {
		fmt.Print(err)
	}
	return y
}

// PutParameters() takes a slice of parameters from SSM and returns
// a secretData map (map[string][]byte).
func (d *secretData) PutParameters(p []*ssm.Parameter) *secretData {
	for _, v := range p {
		value := []byte(*v.Value)
		s := strings.Split(*v.Name, "/")
		name := s[len(s)-1]
		(*d)[name] = value
	}
	return d
}

func (c *Config) readConfig() *Config {
	f, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Print(err)
	}

	err = yaml.Unmarshal(f, c)
	if err != nil {
		fmt.Print(err)
	}

	return c
}

func main() {
	var config Config
	data := make(secretData)
	recursive := false // we don't want to recurse yet... maybe in the future
	decryption := true

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	config.readConfig()

	svc := ssm.New(sess)

	p, err := svc.GetParametersByPath(&ssm.GetParametersByPathInput{
		Path:           &config.Path,
		Recursive:      &recursive,
		WithDecryption: &decryption,
	})
	if err != nil {
		fmt.Print(err)
	}

	data.PutParameters(p.Parameters)

	s := &secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: v1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: config.Metadata.Name,
		},
		Type: v1.SecretTypeOpaque,
		Data: data,
	}
	s.Print() // print the secret resource
}
