package tests_test

import (
	"sigs.k8s.io/kustomize/v3/k8sdeps/kunstruct"
	"sigs.k8s.io/kustomize/v3/k8sdeps/transformer"
	"sigs.k8s.io/kustomize/v3/pkg/fs"
	"sigs.k8s.io/kustomize/v3/pkg/loader"
	"sigs.k8s.io/kustomize/v3/pkg/plugins"
	"sigs.k8s.io/kustomize/v3/pkg/resmap"
	"sigs.k8s.io/kustomize/v3/pkg/resource"
	"sigs.k8s.io/kustomize/v3/pkg/target"
	"sigs.k8s.io/kustomize/v3/pkg/validators"
	"testing"
)

func writeMetadataOverlaysExternalMysql(th *KustTestHarness) {
	th.writeF("/manifests/metadata/overlays/external-mysql/metadata-deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deployment
  labels:
    component: server
spec:
  replicas: 1
  selector:
    matchLabels:
      component: server
  template:
    metadata:
      labels:
        component: server
    spec:
      containers:
      - name: container
        envFrom:
          - configMapRef:
              name: metadata-db-parameters
          - secretRef:
              name: metadata-db-secrets
        command: ["./server/server",
                  "--http_port=8080",
                  "--mysql_service_host=$(MYSQL_HOST)",
                  "--mysql_service_port=$(MYSQL_PORT)",
                  "--mysql_service_user=$(MYSQL_USERNAME)",
                  "--mysql_service_password=$(MYSQL_PASSWORD)",
                  "--mlmd_db_name=$(MYSQL_DATABASE)"]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grpc-deployment
  labels:
    component: grpc-server
spec:
  replicas: 1
  selector:
    matchLabels:
      component: grpc-server
  template:
    metadata:
      labels:
        component: grpc-server
    spec:
      containers:
        - name: container
          envFrom:
          - configMapRef:
              name: metadata-db-parameters
          - secretRef:
              name: metadata-db-secrets
          - configMapRef:
              name: metadata-grpc-configmap
          args: ["--grpc_port=$(METADATA_GRPC_SERVICE_PORT)",
                 "--mysql_config_host=$(MYSQL_HOST)",
                 "--mysql_config_database=$(MYSQL_DATABASE)",
                 "--mysql_config_port=$(MYSQL_PORT)",
                 "--mysql_config_user=$(MYSQL_USERNAME)",
                 "--mysql_config_password=$(MYSQL_PASSWORD)"
          ]
`)
	th.writeF("/manifests/metadata/overlays/external-mysql/params.env", `
MYSQL_HOST=external_host
MYSQL_DATABASE=metadb
MYSQL_PORT=3306
MYSQL_ALLOW_EMPTY_PASSWORD=true
`)
	th.writeF("/manifests/metadata/overlays/external-mysql/secrets.env", `
MYSQL_USERNAME=root
MYSQ_PASSWORD=test
`)
	th.writeK("/manifests/metadata/overlays/external-mysql", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
commonLabels:
  kustomize.component: metadata
configMapGenerator:
- name: metadata-db-parameters
  env: params.env
secretGenerator:
- name: metadata-db-secrets
  env: secrets.env
bases:
- ../../base
patchesStrategicMerge:
- metadata-deployment.yaml
`)
	th.writeF("/manifests/metadata/base/metadata-deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deployment
  labels:
    component: server
spec:
  replicas: 1
  selector:
    matchLabels:
      component: server
  template:
    metadata:
      labels:
        component: server
      annotations:
        sidecar.istio.io/inject: "false"
    spec:
      containers:
      - name: container
        image: gcr.io/kubeflow-images-public/metadata:v0.1.11
        command: ["./server/server",
                  "--http_port=8080"]
        ports:
        - name: backendapi
          containerPort: 8080

        readinessProbe:
          httpGet:
            path: /api/v1alpha1/artifact_types
            port: backendapi
            httpHeaders:
            - name: ContentType
              value: application/json
          initialDelaySeconds: 3
          periodSeconds: 5
          timeoutSeconds: 2
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grpc-deployment
  labels:
    component: grpc-server
spec:
  replicas: 1
  selector:
    matchLabels:
      component: grpc-server
  template:
    metadata:
      labels:
        component: grpc-server
      annotations:
        sidecar.istio.io/inject: "false"
    spec:
      containers:
        - name: container
          envFrom:
          - configMapRef:
              name: metadata-grpc-configmap
          image: gcr.io/tfx-oss-public/ml_metadata_store_server:v0.21.1
          command: ["/bin/metadata_store_server"]
          args: ["--grpc_port=$(METADATA_GRPC_SERVICE_PORT)"]
          ports:
            - name: grpc-backendapi
              containerPort: 8080 #The value of the port number needs to be in sync with value  specified in grpc-params.env
`)
	th.writeF("/manifests/metadata/base/metadata-service.yaml", `
kind: Service
apiVersion: v1
metadata:
  labels:
    app: metadata
  name: service
spec:
  selector:
    component: server
  type: ClusterIP
  ports:
  - port: 8080
    protocol: TCP
    name: backendapi
---
kind: Service
apiVersion: v1
metadata:
  labels:
    app: grpc-metadata
  name: grpc-service
spec:
  selector:
    component: grpc-server
  type: ClusterIP
  ports:
    - port: 8080
      protocol: TCP
      name: grpc-backendapi
`)
	th.writeF("/manifests/metadata/base/metadata-ui-deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ui
  labels:
    app: metadata-ui
spec:
  selector:
    matchLabels:
      app: metadata-ui
  template:
    metadata:
      name: ui
      labels:
        app: metadata-ui
      annotations:
        sidecar.istio.io/inject: "false"
    spec:
      containers:
      - image: gcr.io/kubeflow-images-public/metadata-frontend:v0.1.8
        imagePullPolicy: IfNotPresent
        name: metadata-ui
        ports:
        - containerPort: 3000
      serviceAccountName: ui

`)
	th.writeF("/manifests/metadata/base/metadata-ui-role.yaml", `
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: Role
metadata:
  labels:
    app: metadata-ui
  name: ui
rules:
- apiGroups:
  - ""
  resources:
  - pods
  - pods/log
  verbs:
  - create
  - get
  - list
- apiGroups:
  - "kubeflow.org"
  resources:
  - viewers
  verbs:
  - create
  - get
  - list
  - watch
  - delete
`)
	th.writeF("/manifests/metadata/base/metadata-ui-rolebinding.yaml", `
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: RoleBinding
metadata:
  labels:
    app: metadata-ui
  name: ui
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: ui
subjects:
- kind: ServiceAccount
  name: ui
  namespace: kubeflow
`)
	th.writeF("/manifests/metadata/base/metadata-ui-sa.yaml", `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ui
`)
	th.writeF("/manifests/metadata/base/metadata-ui-service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: ui
  labels:
    app: metadata-ui
spec:
  ports:
  - port: 80
    targetPort: 3000
  selector:
    app: metadata-ui
`)
	th.writeF("/manifests/metadata/base/metadata-envoy-deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: envoy-deployment
  labels:
    component: envoy
spec:
  replicas: 1
  selector:
    matchLabels:
      component: envoy
  template:
    metadata:
      labels:
        component: envoy
      annotations:
        sidecar.istio.io/inject: "false"
    spec:
      containers:
      - name: container
        image: gcr.io/ml-pipeline/envoy:metadata-grpc
        ports:
        - name: md-envoy
          containerPort: 9090
        - name: envoy-admin
          containerPort: 9901
`)
	th.writeF("/manifests/metadata/base/metadata-envoy-service.yaml", `
kind: Service
apiVersion: v1
metadata:
  labels:
    app: metadata
  name: envoy-service
spec:
  selector:
    component: envoy
  type: ClusterIP
  ports:
  - port: 9090
    protocol: TCP
    name: md-envoy
`)
	th.writeF("/manifests/metadata/base/params.env", `
uiClusterDomain=cluster.local
`)
	th.writeF("/manifests/metadata/base/grpc-params.env", `
METADATA_GRPC_SERVICE_HOST=metadata-grpc-service
METADATA_GRPC_SERVICE_PORT=8080
`)
	th.writeK("/manifests/metadata/base", `
namePrefix: metadata-
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
commonLabels:
  kustomize.component: metadata
configMapGenerator:
- name: ui-parameters
  env: params.env
- name: metadata-grpc-configmap
  env: grpc-params.env
resources:
- metadata-deployment.yaml
- metadata-service.yaml
- metadata-ui-deployment.yaml
- metadata-ui-role.yaml
- metadata-ui-rolebinding.yaml
- metadata-ui-sa.yaml
- metadata-ui-service.yaml
- metadata-envoy-deployment.yaml
- metadata-envoy-service.yaml
namespace: kubeflow
vars:
- name: ui-namespace
  objref:
    kind: Service
    name: ui
    apiVersion: v1
  fieldref:
    fieldpath: metadata.namespace
- name: ui-clusterDomain
  objref:
    kind: ConfigMap
    name: ui-parameters
    version: v1
  fieldref:
    fieldpath: data.uiClusterDomain
- name: service
  objref:
    kind: Service
    name: ui
    apiVersion: v1
  fieldref:
    fieldpath: metadata.name
- name: metadata-envoy-service
  objref:
    kind: Service
    name: envoy-service
    apiVersion: v1
  fieldref:
    fieldpath: metadata.name
images:
- name: gcr.io/kubeflow-images-public/metadata
  newName: gcr.io/kubeflow-images-public/metadata
  newTag: v0.1.11
- name: gcr.io/tfx-oss-public/ml_metadata_store_server
  newName: gcr.io/tfx-oss-public/ml_metadata_store_server
  newTag: v0.21.1
- name: gcr.io/ml-pipeline/envoy
  newName: gcr.io/ml-pipeline/envoy
  newTag: metadata-grpc
- name: mysql
  newName: mysql
  newTag: 8.0.3
- name: gcr.io/kubeflow-images-public/metadata-frontend
  newName: gcr.io/kubeflow-images-public/metadata-frontend
  newTag: v0.1.8
`)
}

func TestMetadataOverlaysExternalMysql(t *testing.T) {
	th := NewKustTestHarness(t, "/manifests/metadata/overlays/external-mysql")
	writeMetadataOverlaysExternalMysql(th)
	m, err := th.makeKustTarget().MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	expected, err := m.AsYaml()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	targetPath := "../metadata/overlays/external-mysql"
	fsys := fs.MakeRealFS()
	lrc := loader.RestrictionRootOnly
	_loader, loaderErr := loader.NewLoader(lrc, validators.MakeFakeValidator(), targetPath, fsys)
	if loaderErr != nil {
		t.Fatalf("could not load kustomize loader: %v", loaderErr)
	}
	rf := resmap.NewFactory(resource.NewFactory(kunstruct.NewKunstructuredFactoryImpl()), transformer.NewFactoryImpl())
	pc := plugins.DefaultPluginConfig()
	kt, err := target.NewKustTarget(_loader, rf, transformer.NewFactoryImpl(), plugins.NewLoader(pc, rf))
	if err != nil {
		th.t.Fatalf("Unexpected construction error %v", err)
	}
	actual, err := kt.MakeCustomizedResMap()
	if err != nil {
		t.Fatalf("Err: %v", err)
	}
	th.assertActualEqualsExpected(actual, string(expected))
}
