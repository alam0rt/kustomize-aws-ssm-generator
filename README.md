## Parameter Store Generator for Kustomize

Parameter Store Generator is a plugin for Kustomize

It takes a config file like:
```yaml
apiVersion: k8s.samlockart.com/v1
kind: ParameterStore
metadata:
  name: example
path: /example/path
```

and will generate a Secret resource named `example` from the AWS Parameter Store path  `/example/path`


#### Installation

> TODO: create Makefile and better instructions

Installation is pretty straight forward, it goes:

* Compile the binary
* Copy the binary to `$XDG_CONFIG_HOME/kustomize/plugin/k8s.samlockart.com/v1/parameterstore/ParameterStore`
* Define a `generator` inside your kustomization and point it to something like [config.yaml](./config.yaml)
* Build with alpha plugin feature enabled: `kustomize build --enable_alpha_plugins`
