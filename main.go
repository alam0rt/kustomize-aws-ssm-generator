package main

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"

	"encoding/json"

	"github.com/ghodss/yaml"

	"io/ioutil"
	"strings"

	"strconv"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIVERSION is the same as the `apiVersion` + /v1
const APIVERSION = "k8s.samlockart.com"

var (
	versioned             bool // do we care about the parameter version?
	config                Config
	data                  secretData
	recursive, decryption bool
	svc                   *ssm.SSM
	sess                  *session.Session
	parameters            *ssm.GetParametersByPathOutput
	annotations           map[string]string
)

// secretData is a map holding the secret resource data.
// The reason it being a type is so we can add methods to it easily.
type secretData map[string][]byte

// Ditto
type secret v1.Secret

// Config is a structure which holds the config map
// data. The config is passed to the application as a path
// and is then unmarshalled into this struct.
type Config struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	}
	Path     string `yaml:"path"`
	Version  int64  `yaml:"version,omitempty"`
	Annotate bool   `yaml:"annotate,omitempty"`
	Region   string `yaml:"region"`
}

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
		// if we have set the version field, we will only use that version of the
		// parameter.
		if *v.Version == config.Version || versioned == false {
			value := []byte(*v.Value)        // get parameter value as []byte
			s := strings.Split(*v.Name, "/") // split path up and choose last element for name
			name := s[len(s)-1]
			(*d)[name] = value
		}
	}
	return d
}

func (s *secret) GenAnnotations() *secret {
	// if we want to annotate the resource, this is where we do it
	annotations = make(map[string]string)
	annotations[APIVERSION+"/paramPath"] = config.Path // TODO: clean this up, add option in cofing to include
	annotations[APIVERSION+"/paramRegion"] = config.Region
	if versioned {
		annotations[APIVERSION+"/paramVersion"] = strconv.Itoa(int(config.Version))
	}
	s.SetAnnotations(annotations)

	return s
}

// Panic will print provided string and exit
func Panic(s string) {
	err := fmt.Errorf(s)
	fmt.Println(err)
	os.Exit(1)
}

func (c *Config) readConfig() *Config {
	if len(os.Args) == 1 {
		Panic("no config file - the first and only argument must be a path to a valid config")
	}
	f, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		Panic("enable to read config file")
	}

	err = yaml.Unmarshal(f, c)
	if err != nil {
		Panic("config file is invalid")
	}

	return c
}

func init() {
	config.readConfig()

	if config.Version == 0 {
		versioned = false
	} else {
		versioned = true
	}

	data = make(secretData)
	recursive = false // we don't want to recurse yet... maybe in the future
	decryption = true

	sess = session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	svc = ssm.New(sess, &aws.Config{
		MaxRetries: aws.Int(3),
		Region:     &config.Region,
	})
}

func (d *secretData) getSecrets() (*secretData, error) {
	input := &ssm.GetParametersByPathInput{
		Path:           &config.Path,
		Recursive:      &recursive,
		WithDecryption: &decryption,
	}
	for {
		resp, err := svc.GetParametersByPath(input)
		if err != nil {
			return nil, err
		}

		d.PutParameters(resp.Parameters)
		if resp.NextToken == nil {
			break
		}
		input.SetNextToken(*resp.NextToken)
	}
	if len(*d) == 0 {
		Panic("there was a problem creating a list of secrets: no secrets found")
	}
	return d, nil
}

func main() {
	// populate map with secrets from SSM
	data.getSecrets()

	// create a secret resource
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

	if config.Annotate {
		s.GenAnnotations() // populate with annotations if set
	}
	s.Print() // print the secret resource
}
