## Parameter Store Generator for Kustomize

Parameter Store Generator is a plugin for Kustomize

It takes a config file like:
```yaml
apiVersion: k8s.samlockart.com/v1
kind: ParameterStore
metadata:
  name: example
path: /example/path
version: 1
annotate: true
```

and will generate a Secret resource named `example` from the AWS Parameter Store path  `/example/path`

#### Fields

*path* is the AWS SSM parameter path, this is fed to ssm:getparametersbypath
*version* is the parameter version you want to retrieve from SSM (optional, if not set, latest parameter will be used.)
*annotate* is a bool, if set to true the `v1/Secret` resource will be annotated with some information about where the parameters were pulled from and what version (if any) are used


#### Installation

> TODO: create Makefile and better instructions

Installation is pretty straight forward, it goes:

* Compile the binary
* Copy the binary to `$XDG_CONFIG_HOME/kustomize/plugin/k8s.samlockart.com/v1/parameterstore/ParameterStore`
* Define a `generator` inside your kustomization and point it to something like [config.yaml](./config.yaml)
* Build with alpha plugin feature enabled: `kustomize build --enable_alpha_plugins`
