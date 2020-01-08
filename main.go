package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"

	"encoding/json"

	"github.com/ghodss/yaml"

	"io/ioutil"
	"strings"

	"strconv"

	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIVERSION is the same as the `apiVersion` + /v1
const APIVERSION = "k8s.samlockart.com"

var (
	sess *session.Session
	_    Store = &SSMStore{}
)

type Store interface {
	List(path string) ([]Secret, error)
}

type Secret struct {
	Value *string
	Meta  SecretMetadata
}

type SecretMetadata struct {
	Version int64
	Key     string
}

type Session struct {
	secrets  SecretData
	config   Config
	manifest *manifest
	svc      ssmiface.SSMAPI
}

type Manifest v1.Secret

func (s *Secret) Eat(p *ssm.Parameter) *Secret {
	s.Value = p.Value
	s.Meta.Key = *p.Name
	s.Meta.Version = *p.Version
	return s
}

// List takes a path and returns a slice of Secrets
func (s *SSMStore) List(path string) ([]Secret, error) {
	secrets := map[string]Secret{}
	i := &ssm.GetParametersByPathInput{
		Path:           aws.String(path),
		Recursive:      aws.Bool(false),
		WithDecryption: aws.Bool(true),
	}
	parameterOutput, err := s.svc.GetParametersByPath(i)
	for {
		if err != nil {
			return nil, err
		}

		for _, p := range parameterOutput.Parameters {
			s := &Secret{}
			s.Eat(p)
			secrets[s.Meta.Key] = *s
		}
		if parameterOutput.NextToken == nil {
			break
		}
		i.SetNextToken(*parameterOutput.NextToken)
	}

	return values(secrets), nil
}

func values(m map[string]Secret) []Secret {
	values := []Secret{}
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

func NewSSMStore(region string) (*SSMStore, error) {
	session := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	svc := ssm.New(session, &aws.Config{
		Region:     &region,
		MaxRetries: aws.Int(3),
	})
	return &SSMStore{
		svc: svc,
	}, nil
}

// SecretData is a map holding the secret resource data.
// The reason it being a type is so we can add methods to it easily.
type SecretData map[string][]byte

// Ditto
type manifest v1.Secret

// Config is a structure which holds the config map
// data. The config is passed to the application as a path
// and is then unmarshalled into this struct.
type Config struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	}
	Path           string `yaml:"path"`
	Version        int64  `yaml:"version,omitempty"`
	Annotate       bool   `yaml:"annotate,omitempty"`
	Region         string `yaml:"region"`
	WithDecryption bool
	Versioned      bool
	Recursive      bool
}

func (s *manifest) Print() {
	fmt.Printf("%s\n", s.String())
}

func (s *manifest) String() string {
	b, err := s.Marshal()
	if err != nil {
		Panic(err.Error())
	}
	return string(b)
}

// Marshal() simply marshals the secret resorce into a JSON byte stream
// and then we call JSONToYAML to convert that. The reason being, marshalling
// from struct to YAML does NOT honor the *v1.Secret struct's `json:",omitempty" directives.
// If we don't honor those, we get a large YAML resource with lots of empty fields.
func (s *manifest) Marshal() ([]byte, error) {
	j, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	y, err := yaml.JSONToYAML(j)
	if err != nil {
		return nil, err
	}
	return y, nil
}

// PutSecrets2 takes a slice of parameters from SSM and returns
// a SecretData map (map[string][]byte).
func (s *Session) PutSecrets(p []*ssm.Parameter) error {
	for _, v := range p {
		// if we have set the version field, we will only use that version of the
		// parameter.
		if v.Version == &s.config.Version || s.config.Versioned {
			value := []byte(*v.Value)          // get parameter value as []byte
			sec := strings.Split(*v.Name, "/") // split path up and choose last element for name
			name := sec[len(sec)-1]
			s.secrets[name] = value
		}
	}
	return nil
}

func (s *Session) GenAnnotations() *Session {
	// if we want to annotate the resource, this is where we do it
	s.manifest.Annotations = make(map[string]string)
	s.manifest.Annotations[APIVERSION+"/paramPath"] = s.config.Path // TODO: clean this up, add option in cofing to include
	s.manifest.Annotations[APIVERSION+"/paramRegion"] = s.config.Region
	if s.config.Versioned {
		s.manifest.Annotations[APIVERSION+"/paramVersion"] = strconv.Itoa(int(s.config.Version))
	}

	return s
}

// Panic will print provided string and exit
func Panic(s string) {
	err := fmt.Errorf(s)
	fmt.Println(err)
	os.Exit(1)
}

func (c *Config) readConfig() (*Config, error) {

	if len(os.Args) == 1 {
		return nil, errors.New("you must provide a path to a valid config as the first argument to this application")
	}
	f, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(f, c)
	if err != nil {
		return nil, err
	}

	return c, err
}

// SSMStore implements an SSM service object
type SSMStore struct {
	svc ssmiface.SSMAPI
}

func (s *Session) GetSecretsByPath() error {
	i := &ssm.GetParametersByPathInput{
		Path:           &s.config.Path,
		Recursive:      &s.config.Recursive,
		WithDecryption: &s.config.WithDecryption,
	}
	var p []*ssm.Parameter
	for {
		resp, err := s.svc.GetParametersByPath(i)
		if err != nil {
			return err
		}

		p = append(resp.Parameters)
		if resp.NextToken == nil {
			break
		}
		i.SetNextToken(*resp.NextToken)
	}
	s.PutSecrets(p)
	return nil
}

func main() {
	var omar Store
	omar, err := NewSSMStore("ap-southeast-2")
	wow, _ := omar.List("/ops/esp/7e/dev/reconciliation")
	fmt.Print(wow)
	for _, s := range wow {
		fmt.Print(*s.Value)
	}

	os.Exit(0)
	sess = session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	s := Session{
		secrets: make(SecretData),
	}
	_, err = s.config.readConfig()
	if err != nil {
		Panic(err.Error())
	}
	s.config.Versioned = true
	if s.config.Version == 0 {
		s.config.Versioned = false
	}

	s.svc = ssm.New(sess, &aws.Config{
		MaxRetries: aws.Int(3),
		Region:     &s.config.Region,
	})

	err = s.GetSecretsByPath()
	if err != nil {
		Panic(err.Error())
	}

	// create a secret resource
	s.manifest = &manifest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: v1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: s.config.Metadata.Name,
		},
		Type: v1.SecretTypeOpaque,
		Data: s.secrets,
	}

	if s.config.Annotate {
		s.GenAnnotations()
	}
	s.manifest.Print() // print the secret resource
}
