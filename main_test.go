package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"

	"github.com/stretchr/testify/assert"
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

func createFakeSession(mock *ssmiface.SSMAPI) *Session {
	s := &Session{
		config: Config{
			APIVersion: "k8s.samlockart.com/v1",
			Kind:       "ParameterStore",
			Metadata: metadata{
				Name: "test",
			},
			Path:           "/example/secret",
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

	s.svc = mock
	return s
}

func NewTestSSMStore(mock ssmiface.SSMAPI) *SSMStore {
	return &SSMStore{
		svc: mock,
	}
}

func TestParam(t *testing.T) {
	t.Run("region field must be obeyed", func(t *testing.T) {
		s := createFakeSession()
		//	svc := &mockSSMClient{}
		assert.Equal(t, "ap-southeast-2", aws.StringValue(&s.config.Region))
		svc := &mockSSMClient{
			parameters: make(map[string]mockParameter),
		}
		v := "test"
		n := "/example/secret"
		secstring := ssm.ParameterTypeSecureString
		svc.PutParameter(&ssm.PutParameterInput{
			Value: &v,
			Name:  &s.config.Path,
			Type:  &secstring,
		})
		fmt.Print(svc.parameters[n].currentParam)

		err := s.GetSecretsByPath()
		if err != nil {
			fmt.Print(err)
		}

	})
}
