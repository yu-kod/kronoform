load('ext://restart_process', 'docker_build_with_restart')

DOCKERFILE = '''FROM golang:alpine
WORKDIR /
COPY ./bin/manager /
CMD ["/manager"]
'''

def manifests():
    return './bin/controller-gen crd rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases;'

def generate():
    return './bin/controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./...";'

def binary():
    return 'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/manager cmd/main.go'

def plugin_binary():
    return 'go build -o bin/kubectl-kronoform cmd/kubectl-kronoform/main.go'

# Generate manifests and go files
local_resource('make manifests', manifests(), deps=["api", "internal", "hooks"], ignore=['*/*/zz_generated.deepcopy.go'])
local_resource('make generate', generate(), deps=["api", "hooks"], ignore=['*/*/zz_generated.deepcopy.go'])

# Deploy CRD
local_resource(
    'CRD', manifests() + 'kustomize build config/crd | kubectl apply -f -', deps=["api"],
    ignore=['*/*/zz_generated.deepcopy.go'])

# Deploy manager
watch_file('./config/')
k8s_yaml(kustomize('./config/dev'))

local_resource(
    'Watch & Compile', generate() + binary(), deps=['internal', 'api', 'cmd/main.go'],
    ignore=['*/*/zz_generated.deepcopy.go'])

# Build kubectl plugin
local_resource(
    'Plugin Build', plugin_binary(), deps=['cmd/kubectl-kronoform', 'api'],
    ignore=['*/*/zz_generated.deepcopy.go'])

docker_build_with_restart(
    'controller:latest', '.',
    dockerfile_contents=DOCKERFILE,
    entrypoint=['/manager'],
    only=['./bin/manager'],
    platform='linux/amd64',
    live_update=[
        sync('./bin/manager', '/manager'),
    ]
)

local_resource(
    'Sample', 'kubectl apply -f ./config/samples/history_v1alpha1_kronoform.yaml',
    deps=["./config/samples/history_v1alpha1_kronoform.yaml"])

# Test history tracking functionality
local_resource(
    'Test ConfigMap', './bin/kubectl-kronoform apply -f ./config/samples/configmap_example.yaml',
    deps=["./bin/kubectl-kronoform", "./config/samples/configmap_example.yaml"],
    resource_deps=['Plugin Build', 'CRD'])

# Test with Deployment example
local_resource(
    'Test Deployment', './bin/kubectl-kronoform apply -f ./config/samples/deployment_example.yaml',
    deps=["./bin/kubectl-kronoform", "./config/samples/deployment_example.yaml"],
    resource_deps=['Plugin Build', 'CRD'],
    trigger_mode=TRIGGER_MODE_MANUAL)

# View history records
local_resource(
    'Show History', 'kubectl get kronoformhistories',
    resource_deps=['Test ConfigMap'],
    trigger_mode=TRIGGER_MODE_MANUAL)