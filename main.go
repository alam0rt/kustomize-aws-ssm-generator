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

// Ensure that SSMStore satisfies the Store interface
var _ Store = &SSMStore{}

// SSMStore implements an SSM service object
type SSMStore struct {
	svc ssmiface.SSMAPI
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
	Type           string `yaml:"type,omitempty"`
	WithDecryption bool
	Versioned      bool
	Recursive      bool
}

type Store interface {
	Get(path string) ([]Secret, error)
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
	store    Store
	manifest *manifest
}

func matchType(s v1.SecretType) bool {
	var secret v1.SecretType = v1.SecretType(s)
	secretTypes := []v1.SecretType{
		v1.SecretTypeBasicAuth,
		v1.SecretTypeBootstrapToken,
		v1.SecretTypeDockerConfigJson,
		v1.SecretTypeDockercfg,
		v1.SecretTypeOpaque,
		v1.SecretTypeSSHAuth,
		v1.SecretTypeServiceAccountToken,
		v1.SecretTypeTLS,
	}

	for _, t := range secretTypes {
		if t == secret {
			return true
		}
	}
	return false

}

func (s *Session) RenderManifest(secrets []Secret) error {
	data := make(map[string][]byte)
	for _, sec := range secrets {
		if sec.Meta.Version == s.config.Version || s.config.Versioned == false {
			data[sec.Name()] = []byte(*sec.Value)
		}
	}
	if len(data) == 0 {
		err := errors.New("no secrets retrieved - cannot create Secret")
		return err

	}

	// if the provided type matches a known type, use it
	// otherwise just let K8s decide
	t := v1.SecretType(s.config.Type)
	if !matchType(t) {
		t = ""
	}

	s.manifest = &manifest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: v1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: s.config.Metadata.Name,
		},
		Type: t,
		Data: data,
	}

	if s.config.Annotate {
		s.GenAnnotations()
	}
	fmt.Print(s.manifest)
	return nil

}

// Name returns the name of a given Secret type
func (s *Secret) Name() string {
	n := strings.Split(s.Meta.Key, "/") // split path up and choose last element for name
	return n[len(n)-1]

}

// Put takes an SSM parameter and returns a Secret
func (s *Secret) Put(p *ssm.Parameter) *Secret {
	s.Value = p.Value
	s.Meta.Key = *p.Name
	s.Meta.Version = *p.Version
	return s
}

// Get takes a path and returns a slice of Secrets
func (s *SSMStore) Get(path string) ([]Secret, error) {
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
			s.Put(p)
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

// NewSSMStore takes a region and returns an SSM session
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

// GenAnnotations generates and annotates the Secret resource
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

	if c.Version > 0 {
		c.Versioned = true
	}

	return c, err
}

func main() {
	var s Session
	var secrets []Secret
	_, err := s.config.readConfig()
	if err != nil {
		Panic(err.Error())
	}
	s.store, err = NewSSMStore(s.config.Region)
	if err != nil {
		Panic(err.Error())
	}
	secrets, err = s.store.Get(s.config.Path)
	if err != nil {
		Panic(err.Error())
	}
	err = s.RenderManifest(secrets)
	if err != nil {
		Panic(err.Error())
	}

	os.Exit(0)
}
