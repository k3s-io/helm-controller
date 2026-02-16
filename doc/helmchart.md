# API Reference

## Packages
- [helm.cattle.io/v1](#helmcattleiov1)


## helm.cattle.io/v1






#### FailurePolicy

_Underlying type:_ _string_



_Validation:_
- Enum: [abort reinstall]

_Appears in:_
- [HelmChartConfigSpec](#helmchartconfigspec)
- [HelmChartSpec](#helmchartspec)



#### HelmChart



HelmChart represents configuration and state for the deployment of a Helm chart.



_Appears in:_
- [HelmChartList](#helmchartlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[HelmChartSpec](#helmchartspec)_ |  |  |  |
| `status` _[HelmChartStatus](#helmchartstatus)_ |  |  |  |


#### HelmChartCondition







_Appears in:_
- [HelmChartStatus](#helmchartstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[HelmChartConditionType](#helmchartconditiontype)_ | Type of job condition. |  |  |
| `status` _[ConditionStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#conditionstatus-v1-core)_ | Status of the condition, one of True, False, Unknown. |  |  |
| `reason` _string_ | (brief) reason for the condition's last transition. |  |  |
| `message` _string_ | Human readable message indicating details about last transition. |  |  |


#### HelmChartConditionType

_Underlying type:_ _string_





_Appears in:_
- [HelmChartCondition](#helmchartcondition)

| Field | Description |
| --- | --- |
| `JobCreated` |  |
| `Failed` |  |


#### HelmChartConfig



HelmChartConfig represents additional configuration for the installation of Helm chart release.
This resource is intended for use when additional configuration needs to be passed to a HelmChart
that is managed by an external system.



_Appears in:_
- [HelmChartConfigList](#helmchartconfiglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[HelmChartConfigSpec](#helmchartconfigspec)_ |  |  |  |




#### HelmChartConfigSpec



HelmChartConfigSpec represents additional user-configurable details of an installed and configured Helm chart release.
These fields are merged with or override the corresponding fields on the related HelmChart resource.



_Appears in:_
- [HelmChartConfig](#helmchartconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `values` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#json-v1-apiextensions-k8s-io)_ | Override complex Chart values via structured YAML. Takes precedence over options set via valuesContent.<br />Helm CLI positional argument/flag: `--values` |  |  |
| `valuesContent` _string_ | Override complex Chart values via inline YAML content.<br />Helm CLI positional argument/flag: `--values` |  |  |
| `valuesSecrets` _[SecretSpec](#secretspec) array_ | Override complex Chart values via references to external Secrets.<br />Helm CLI positional argument/flag: `--values` |  |  |
| `failurePolicy` _[FailurePolicy](#failurepolicy)_ | Configures handling of failed chart installation or upgrades.<br />- `reinstall` will perform a clean uninstall and reinstall of the chart.<br />- `abort` will take no action and leave the chart in a failed state so that the administrator can manually resolve the error. | reinstall | Enum: [abort reinstall] <br /> |




#### HelmChartSpec



HelmChartSpec represents the user-configurable details for installation and upgrade of a Helm chart release.



_Appears in:_
- [HelmChart](#helmchart)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `targetNamespace` _string_ | Helm Chart target namespace.<br />Helm CLI positional argument/flag: `--namespace` |  |  |
| `createNamespace` _boolean_ | Create target namespace if not present.<br />Helm CLI positional argument/flag: `--create-namespace` |  |  |
| `chart` _string_ | Helm Chart name in repository, or complete HTTPS URL to chart archive (.tgz)<br />Helm CLI positional argument/flag: `CHART` |  |  |
| `version` _string_ | Helm Chart version. Only used when installing from repository; ignored when .spec.chart or .spec.chartContent is used to install a specific chart archive.<br />Helm CLI positional argument/flag: `--version` |  |  |
| `repo` _string_ | Helm Chart repository URL.<br />Helm CLI positional argument/flag: `--repo` |  |  |
| `repoCA` _string_ | Verify certificates of HTTPS-enabled servers using this CA bundle. Should be a string containing one or more PEM-encoded CA Certificates.<br />Helm CLI positional argument/flag: `--ca-file` |  |  |
| `repoCAConfigMap` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core)_ | Reference to a ConfigMap containing CA Certificates to be be trusted by Helm. Can be used along with or instead of `.spec.repoCA`<br />Helm CLI positional argument/flag: `--ca-file` |  |  |
| `set` _object (keys:string, values:[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#intorstring-intstr-util))_ | Override simple Chart values. These take precedence over options set via values or valuesContent.<br />Helm CLI positional argument/flag: `--set`, `--set-string` |  |  |
| `values` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#json-v1-apiextensions-k8s-io)_ | Override complex Chart values via structured YAML. Takes precedence over options set via valuesContent.<br />Helm CLI positional argument/flag: `--values` |  |  |
| `valuesContent` _string_ | Override complex Chart values via inline YAML content.<br />Helm CLI positional argument/flag: `--values` |  |  |
| `valuesSecrets` _[SecretSpec](#secretspec) array_ | Override complex Chart values via references to external Secrets.<br />Helm CLI positional argument/flag: `--values` |  |  |
| `helmVersion` _string_ | DEPRECATED. Helm version to use. Only v3 is currently supported. |  |  |
| `bootstrap` _boolean_ | Set to True if this chart is needed to bootstrap the cluster (Cloud Controller Manager, CNI, etc). |  |  |
| `takeOwnership` _boolean_ | Set to True if helm should take ownership of existing resources when installing/upgrading the chart.<br />Helm CLI positional argument/flag: `--take-ownership` |  |  |
| `chartContent` _string_ | Base64-encoded chart archive .tgz; overides `.spec.chart` and `.spec.version`.<br />Helm CLI positional argument/flag: `CHART` |  |  |
| `jobImage` _string_ | Specify the image to use for tht helm job pod when installing or upgrading the helm chart. |  |  |
| `backOffLimit` _integer_ | Specify the number of retries before considering the helm job failed. |  |  |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#duration-v1-meta)_ | Timeout for Helm operations.<br />Helm CLI positional argument/flag: `--timeout` |  |  |
| `failurePolicy` _[FailurePolicy](#failurepolicy)_ | Configures handling of failed chart installation or upgrades.<br />- `reinstall` will perform a clean uninstall and reinstall of the chart.<br />- `abort` will take no action and leave the chart in a failed state so that the administrator can manually resolve the error. | reinstall | Enum: [abort reinstall] <br /> |
| `authSecret` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core)_ | Reference to Secret of type kubernetes.io/basic-auth holding Basic auth credentials for the Chart repo. |  |  |
| `authPassCredentials` _boolean_ | Pass Basic auth credentials to all domains.<br />Helm CLI positional argument/flag: `--pass-credentials` |  |  |
| `insecureSkipTLSVerify` _boolean_ | Skip TLS certificate checks for the chart download.<br />Helm CLI positional argument/flag: `--insecure-skip-tls-verify` |  |  |
| `plainHTTP` _boolean_ | Use insecure HTTP connections for the chart download.<br />Helm CLI positional argument/flag: `--plain-http` |  |  |
| `dockerRegistrySecret` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#localobjectreference-v1-core)_ | Reference to Secret of type kubernetes.io/dockerconfigjson holding Docker auth credentials for the OCI-based registry acting as the Chart repo. |  |  |
| `podSecurityContext` _[PodSecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podsecuritycontext-v1-core)_ | Custom PodSecurityContext for the helm job pod. |  |  |
| `securityContext` _[SecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#securitycontext-v1-core)_ | custom SecurityContext for the helm job pod. |  |  |


#### HelmChartStatus



HelmChartStatus represents the resulting state from processing HelmChart events



_Appears in:_
- [HelmChart](#helmchart)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `jobName` _string_ | The name of the job created to install or upgrade the chart. |  |  |
| `conditions` _[HelmChartCondition](#helmchartcondition) array_ | `JobCreated` indicates that a job has been created to install or upgrade the chart.<br />`Failed` indicates that the helm job has failed and the failure policy is set to `abort`. |  |  |


#### SecretSpec



SecretSpec describes a key in a secret to load chart values from.



_Appears in:_
- [HelmChartConfigSpec](#helmchartconfigspec)
- [HelmChartSpec](#helmchartspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the secret. Must be in the same namespace as the HelmChart resource. |  |  |
| `keys` _string array_ | Keys to read values content from. If no keys are specified, the secret is not used. |  |  |
| `ignoreUpdates` _boolean_ | Ignore changes to the secret, and mark the secret as optional.<br />By default, the secret must exist, and changes to the secret will trigger an upgrade of the chart to apply the updated values.<br />If `ignoreUpdates` is true, the secret is optional, and changes to the secret will not trigger an upgrade of the chart. |  |  |


