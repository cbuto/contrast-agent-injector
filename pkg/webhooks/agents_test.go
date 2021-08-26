package webhooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

func TestGeneratePatches(t *testing.T) {
	podYaml := `
apiVersion: v1
kind: Pod
metadata:
  name: webgoat-pod
  labels:
    app: webgoat
  annotations:
    contrast-agent-injector/language: java
    contrast-agent-injector/version: 3.8.7.21531
    contrast-agent-injector/enabled: "true"
spec:
  containers:
  - name: webgoat
    image: webgoat/webgoat-8.0
    ports:
    - containerPort: 8080
    env:
      - name: EXAMPLE_VAR
        value: test
`
	scheme := runtime.NewScheme()
	codecFactory := serializer.NewCodecFactory(scheme)
	deserializer := codecFactory.UniversalDeserializer()

	podObject, _, err := deserializer.Decode([]byte(podYaml), nil, &corev1.Pod{})
	assert.NoError(t, err)
	pod := podObject.(*corev1.Pod)

	agentPatch := AgentPatch{
		pod:        *pod,
		secretName: "test",
	}

	patches, err := agentPatch.GenerateAgentPatches()
	assert.NoError(t, err)
	assert.Equal(t, 8, len(patches))
}

func TestGeneratePatchesUnsupportedLanguage(t *testing.T) {
	podYaml := `
apiVersion: v1
kind: Pod
metadata:
  name: webgoat-pod
  labels:
    app: webgoat
  annotations:
    contrast-agent-injector/language: python
    contrast-agent-injector/version: 3.8.7.21531
    contrast-agent-injector/enabled: "true"
spec:
  containers:
  - name: flask
    image: flask
    ports:
    - containerPort: 8080
    env:
      - name: EXAMPLE_VAR
        value: test
`
	scheme := runtime.NewScheme()
	codecFactory := serializer.NewCodecFactory(scheme)
	deserializer := codecFactory.UniversalDeserializer()

	podObject, _, err := deserializer.Decode([]byte(podYaml), nil, &corev1.Pod{})
	assert.NoError(t, err)
	pod := podObject.(*corev1.Pod)

	agentPatch := AgentPatch{
		pod:        *pod,
		secretName: "test",
	}

	_, err = agentPatch.GenerateAgentPatches()
	assert.Error(t, err)
}

func TestGeneratePatchesWithConfig(t *testing.T) {
	podYaml := `
apiVersion: v1
kind: Pod
metadata:
  name: webgoat-pod
  labels:
    app: webgoat
  annotations:
    contrast-agent-injector/language: java
    contrast-agent-injector/version: 3.8.7.21531
    contrast-agent-injector/enabled: "true"
    contrast-agent-injector/config: CONTRAST__SERVER__ENVIRONMENT=qa, CONTRAST__SERVER__NAME=webgoat-k8s
spec:
  containers:
  - name: webgoat
    image: webgoat/webgoat-8.0
    ports:
    - containerPort: 8080
    env:
      - name: EXAMPLE_VAR
        value: test
`
	scheme := runtime.NewScheme()
	codecFactory := serializer.NewCodecFactory(scheme)
	deserializer := codecFactory.UniversalDeserializer()

	podObject, _, err := deserializer.Decode([]byte(podYaml), nil, &corev1.Pod{})
	assert.NoError(t, err)
	pod := podObject.(*corev1.Pod)

	agentPatch := AgentPatch{
		pod:        *pod,
		secretName: "test",
	}

	patches, err := agentPatch.GenerateAgentPatches()

	assert.NoError(t, err)

	assert.Equal(t, 10, len(patches))

	tt := []struct {
		name   string
		envVar corev1.EnvVar
	}{
		{
			name: "Server Environment",
			envVar: corev1.EnvVar{
				Name:  "CONTRAST__SERVER__ENVIRONMENT",
				Value: "qa",
			},
		},
		{
			name: "Server Name",
			envVar: corev1.EnvVar{
				Name:  "CONTRAST__SERVER__NAME",
				Value: "webgoat-k8s",
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			for _, patch := range patches {
				if patch.Path == "/spec/containers/0/env/-" {
					actualEnvVar := patch.Value.(corev1.EnvVar)
					if actualEnvVar.Name == tc.envVar.Name {
						assert.Equal(t, tc.envVar, actualEnvVar)
					}
				}
			}
		})
	}
}

func TestGeneratePatchesDuplicates(t *testing.T) {
	podYaml := `
apiVersion: v1
kind: Pod
metadata:
  name: webgoat-pod
  labels:
    app: webgoat
  annotations:
    contrast-agent-injector/language: java
    contrast-agent-injector/version: 3.8.7.21531
    contrast-agent-injector/enabled: "true"
spec:
  initContainers:
  - args:
    - "echo downloading Contrast agent;\n\t\t\t\tDOWNLOAD_URL_AGENT_JAVA=\"https://repo.maven.apache.org/maven2/com/contrastsecurity/contrast-agent/3.8.7.21531/contrast-agent-3.8.7.21531.jar\";\n\t\t\t\twget
      -q -O /opt/contrast/contrast.jar $DOWNLOAD_URL_AGENT_JAVA;\n\t\t\t\techo finished
      downloading Contrast agent;"
    command:
    - /bin/sh
    - -c
    image: busybox:1.34.0
    imagePullPolicy: IfNotPresent
    name: contrast-agent-injector
    resources: {}
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    volumeMounts:
    - mountPath: /opt/contrast
      name: contrast-agent-injector
  containers:
  - name: webgoat
    image: webgoat/webgoat-8.0
    ports:
    - containerPort: 8080
    env:
      - name: CONTRAST_CONFIG_PATH
        value: test
    volumeMounts:
      - mountPath: /opt/contrast
        name: contrast-agent-injector
      - mountPath: /opt/contrast/contrast_security.yaml
        name: contrast-agent-injector-yaml
        subPath: contrast_security.yaml
  volumes:
    - name: contrast-agent-injector
      emptyDir: {}
    - name: contrast-agent-injector-yaml
      secret:
        secretName: contrast-agent-secret
`
	scheme := runtime.NewScheme()
	codecFactory := serializer.NewCodecFactory(scheme)
	deserializer := codecFactory.UniversalDeserializer()

	podObject, _, err := deserializer.Decode([]byte(podYaml), nil, &corev1.Pod{})
	assert.NoError(t, err)
	pod := podObject.(*corev1.Pod)

	agentPatch := AgentPatch{
		pod:        *pod,
		secretName: "test",
	}

	patches, err := agentPatch.GenerateAgentPatches()
	assert.NoError(t, err)

	assert.Equal(t, 8, len(patches))
}
