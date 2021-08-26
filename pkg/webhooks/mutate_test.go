package webhooks

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	admission "k8s.io/api/admission/v1beta1"
)

func TestMutateHandlerErrors(t *testing.T) {

	mutateConfig := &MutateConfig{
		SecretName: "test",
	}
	testServer := httptest.NewServer(http.HandlerFunc(mutateConfig.MutateHandler))
	defer testServer.Close()

	tt := []struct {
		name        string
		method      string
		body        string
		want        string
		statusCode  int
		contentType string
	}{
		{
			name:        "empty body",
			method:      http.MethodPost,
			body:        ``,
			want:        ``,
			statusCode:  http.StatusBadRequest,
			contentType: jsonContentType,
		},
		{
			name:        "unsupported content type",
			method:      http.MethodPost,
			body:        `test`,
			want:        ``,
			statusCode:  http.StatusUnsupportedMediaType,
			contentType: "text/plain",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(testServer.URL, tc.contentType, strings.NewReader(tc.body))
			assert.NoError(t, err)

			if resp.StatusCode != tc.statusCode {
				t.Errorf("Want status '%d', got '%d'", tc.statusCode, resp.StatusCode)
			}
		})
	}
}

func TestMutateHandlerSuccess(t *testing.T) {
	admissionRequest := `
	{
	  "kind": "AdmissionReview",
	  "apiVersion": "admission.k8s.io/v1beta1",
	  "request": {
		"uid": "6fd1aaea-b081-49ff-9400-89795e8b7556",
		"kind": {
		  "group": "",
		  "version": "v1",
		  "kind": "Pod"
		},
		"resource": {
		  "group": "",
		  "version": "v1",
		  "resource": "pods"
		},
		"namespace": "dummy",
		"operation": "CREATE",
		"userInfo": {
		  "username": "system:serviceaccount:kube-system:replicaset-controller",
		  "uid": "6fd1aaea-b081-49ff-9400-89795e8b7556",
		  "groups": [
			"system:serviceaccounts",
			"system:serviceaccounts:kube-system",
			"system:authenticated"
		  ]
		},
		"object": {
		  "metadata": {
			"generateName": "webgoat-deployment-6c54bd5869-",
			"creationTimestamp": null,
			"labels": {
			  "app": "webgoat"
			},
			"annotations": {
				"contrast-agent-injector/enabled": "true",
				"contrast-agent-injector/language": "java",
				"contrast-agent-injector/version": "3.8.7.21531"
			},
			"ownerReferences": [
			  {
				"apiVersion": "extensions/v1beta1",
				"kind": "ReplicaSet",
				"name": "webgoat-deployment-6c54bd5869",
				"uid": "16c2b355-5f5d-11e8-ac91-36e6bb280816",
				"controller": true,
				"blockOwnerDeletion": true
			  }
			]
		  },
		  "spec": {
			"volumes": [
			  {
				"name": "default-token-tq5lq",
				"secret": {
				  "secretName": "default-token-tq5lq"
				}
			  }
			],
			"containers": [
				{
					"name": "webgoat",
					"image": "webgoat/webgoat-8.0",
					"ports": [{ "containerPort": 8080, "protocol": "TCP" }],
					"env": [{ "name": "EXAMPLE_VAR", "value": "test" }],
					"resources": {},
					"volumeMounts": [],
					"terminationMessagePath": "/dev/termination-log",
					"terminationMessagePolicy": "File",
					"imagePullPolicy": "Always"
				}
			],
			"restartPolicy": "Always",
			"terminationGracePeriodSeconds": 30,
			"dnsPolicy": "ClusterFirst",
			"serviceAccountName": "default",
			"serviceAccount": "default",
			"securityContext": {
			  "seLinuxOptions": {
				"level": "s0:c9,c4"
			  },
			  "fsGroup": 1000080000
			},
			"imagePullSecrets": [],
			"schedulerName": "default-scheduler"
		  },
		  "status": {}
		},
		"oldObject": null
	  }
	}`
	mutateConfig := &MutateConfig{
		SecretName: "test",
	}
	testServer := httptest.NewServer(http.HandlerFunc(mutateConfig.MutateHandler))
	defer testServer.Close()

	resp, err := http.Post(testServer.URL, jsonContentType, strings.NewReader(admissionRequest))
	assert.NoError(t, err)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Want status '%d', got '%d'", http.StatusOK, resp.StatusCode)
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)

	admissionReview := admission.AdmissionReview{}
	err = json.Unmarshal(bodyBytes, &admissionReview)
	assert.NoError(t, err)

	assert.True(t, admissionReview.Response.Allowed)

	patches := []patchOperation{}
	err = json.Unmarshal(admissionReview.Response.Patch, &patches)
	assert.NoError(t, err)

	assert.Equal(t, 8, len(patches))
}

func TestMutateHandlerNotEnabled(t *testing.T) {
	admissionRequest := `
	{
	  "kind": "AdmissionReview",
	  "apiVersion": "admission.k8s.io/v1beta1",
	  "request": {
		"uid": "6fd1aaea-b081-49ff-9400-89795e8b7556",
		"kind": {
		  "group": "",
		  "version": "v1",
		  "kind": "Pod"
		},
		"resource": {
		  "group": "",
		  "version": "v1",
		  "resource": "pods"
		},
		"namespace": "dummy",
		"operation": "CREATE",
		"userInfo": {
		  "username": "system:serviceaccount:kube-system:replicaset-controller",
		  "uid": "6fd1aaea-b081-49ff-9400-89795e8b7556",
		  "groups": [
			"system:serviceaccounts",
			"system:serviceaccounts:kube-system",
			"system:authenticated"
		  ]
		},
		"object": {
		  "metadata": {
			"generateName": "webgoat-deployment-6c54bd5869-",
			"creationTimestamp": null,
			"labels": {
			  "app": "webgoat"
			},
			"annotations": {
				"contrast-agent-injector/language": "java",
				"contrast-agent-injector/version": "3.8.7.21531"
			},
			"ownerReferences": [
			  {
				"apiVersion": "extensions/v1beta1",
				"kind": "ReplicaSet",
				"name": "webgoat-deployment-6c54bd5869",
				"uid": "16c2b355-5f5d-11e8-ac91-36e6bb280816",
				"controller": true,
				"blockOwnerDeletion": true
			  }
			]
		  },
		  "spec": {
			"volumes": [
			  {
				"name": "default-token-tq5lq",
				"secret": {
				  "secretName": "default-token-tq5lq"
				}
			  }
			],
			"containers": [
				{
					"name": "webgoat",
					"image": "webgoat/webgoat-8.0",
					"ports": [{ "containerPort": 8080, "protocol": "TCP" }],
					"env": [{ "name": "EXAMPLE_VAR", "value": "test" }],
					"resources": {},
					"volumeMounts": [],
					"terminationMessagePath": "/dev/termination-log",
					"terminationMessagePolicy": "File",
					"imagePullPolicy": "Always"
				}
			],
			"restartPolicy": "Always",
			"terminationGracePeriodSeconds": 30,
			"dnsPolicy": "ClusterFirst",
			"serviceAccountName": "default",
			"serviceAccount": "default",
			"securityContext": {
			  "seLinuxOptions": {
				"level": "s0:c9,c4"
			  },
			  "fsGroup": 1000080000
			},
			"imagePullSecrets": [],
			"schedulerName": "default-scheduler"
		  },
		  "status": {}
		},
		"oldObject": null
	  }
	}`
	mutateConfig := &MutateConfig{
		SecretName: "test",
	}
	testServer := httptest.NewServer(http.HandlerFunc(mutateConfig.MutateHandler))
	defer testServer.Close()

	resp, err := http.Post(testServer.URL, jsonContentType, strings.NewReader(admissionRequest))
	assert.NoError(t, err)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Want status '%d', got '%d'", http.StatusOK, resp.StatusCode)
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)

	admissionReview := admission.AdmissionReview{}
	err = json.Unmarshal(bodyBytes, &admissionReview)
	assert.NoError(t, err)

	assert.True(t, admissionReview.Response.Allowed)
}
