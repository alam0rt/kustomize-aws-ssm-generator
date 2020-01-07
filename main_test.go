package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockSSM functions stolen from https://github.com/segmentio/chamber/blob/master/store/ssmstore_test.go
type metadata struct {
	Name string `yaml:"name"`
}

type mockSSMClient struct {
	ssmiface.SSMAPI
	parameters map[string]mockParameter
}

type mockParameter struct {
	currentParam *ssm.Parameter
	history      []*ssm.ParameterHistory
	meta         *ssm.ParameterMetadata
}

func (m *mockSSMClient) PutParameter(i *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
	current, ok := m.parameters[*i.Name]
	if !ok {
		current = mockParameter{
			history: []*ssm.ParameterHistory{},
		}
	}

	current.currentParam = &ssm.Parameter{
		Name:  i.Name,
		Type:  i.Type,
		Value: i.Value,
	}
	current.meta = &ssm.ParameterMetadata{
		Description:      i.Description,
		KeyId:            i.KeyId,
		LastModifiedDate: aws.Time(time.Now()),
		LastModifiedUser: aws.String("test"),
		Name:             i.Name,
		Type:             i.Type,
	}
	history := &ssm.ParameterHistory{
		Description:      current.meta.Description,
		KeyId:            current.meta.KeyId,
		LastModifiedDate: current.meta.LastModifiedDate,
		LastModifiedUser: current.meta.LastModifiedUser,
		Name:             current.meta.Name,
		Type:             current.meta.Type,
		Value:            current.currentParam.Value,
	}
	current.history = append(current.history, history)

	m.parameters[*i.Name] = current

	return &ssm.PutParameterOutput{}, nil
}

func (m *mockSSMClient) GetParametersByPath(i *ssm.GetParametersByPathInput) (*ssm.GetParametersByPathOutput, error) {
	parameters := []*ssm.Parameter{}

	for _, param := range m.parameters {

		parameters = append(parameters, param.currentParam)

	}
	return &ssm.GetParametersByPathOutput{
		Parameters: parameters,
		NextToken:  nil,
	}, nil
}

func init() {
	s = Session{
		config: Config{
			APIVersion: "k8s.samlockart.com/v1",
			Kind:       "ParameterStore",
			Metadata: metadata{
				Name: "test",
			},
			Path:           "/",
			Version:        1,
			Annotate:       true,
			Region:         "ap-southeast-2",
			Versioned:      true,
			Recursive:      false,
			WithDecryption: true,
		},
		secrets: make(SecretData),
	}

	if s.config.Version == 0 {
		s.config.Versioned = false
	}

}

func NewTestSSMStore(mock ssmiface.SSMAPI) *SSMStore {
	return &SSMStore{
		svc: mock,
	}
}

func TestParam(t *testing.T) {
	t.Run("region field must be obeyed", func(t *testing.T) {
		s := CreateSSMStore(&s.config)
		assert.Equal(t, "ap-southeast-2", aws.StringValue(s.svc.(*ssm.SSM).Config.Region))
	})
	mock := &mockSSMClient{parameters: map[string]mockParameter{}}
	store := NewTestSSMStore(mock)

	stype := ssm.ParameterTypeSecureString
	_, err := store.svc.PutParameter(&ssm.PutParameterInput{
		Name:  aws.String("/example/path"),
		Value: aws.String("wow"),
		Type:  &stype,
	})
	assert.Nil(t, err)
	data, _ := store.GetSecrets2(&ssm.GetParametersByPathInput{
		Path:           &s.config.Path,
		WithDecryption: &s.config.WithDecryption,
		Recursive:      &s.config.Recursive,
	})

	s.PutSecrets2(data)

	//sd.PutSecrets(data)
	fmt.Print(s.secrets)

}

func TestSecretObject(t *testing.T) {
	name := "test"
	var data SecretData
	s := &secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: v1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: s.config.Metadata.Name,
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
	d := SecretData{
		string("test"): []byte(value),
	}
	s := &secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: v1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: s.config.Metadata.Name,
		},
		Type: v1.SecretTypeOpaque,
		Data: d,
	}

	m, err := s.Marshal()
	if err != nil {
		t.Errorf(err.Error())
	}
	x := &secret{}

	yaml.Unmarshal(m, x)

	if string(x.Data["test"]) != value {
		t.Errorf("There was a problem unmarshalling the secret. Expected secret value of: %s, got: %s", string(x.Data["test"]), value)
	}

}
