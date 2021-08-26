package webhooks

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	admission "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const (
	jsonContentType           = `application/json`
	injectorEnabledAnnotation = `contrast-agent-injector/enabled`
)

var (
	podResource           = metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	universalDeserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
)

// MutateConfig is a struct containing the configuration for the mutation process
type MutateConfig struct {
	SecretName string
}

// patchOperation is an operation of a JSON patch, see https://tools.ietf.org/html/rfc6902 .
type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// MutateHandler is an HTTP handler for handling a admission controller webhook
// and injects the necessary configuration for running the service with a Contrast Agent
func (mutateConfig *MutateConfig) MutateHandler(response http.ResponseWriter, request *http.Request) {
	body, err := ioutil.ReadAll(request.Body)

	if err != nil {
		http.Error(response, "Bad Request", http.StatusBadRequest)

		return
	}

	if len(body) == 0 {
		http.Error(response, "Empty Body", http.StatusBadRequest)

		return
	}

	contentType := request.Header.Get("Content-Type")
	if contentType != jsonContentType {
		http.Error(response, "Invalid Content-Type", http.StatusUnsupportedMediaType)
		log.Error("Invalid Content-Type: ", contentType)
		return
	}

	admissionReviewRequest := admission.AdmissionReview{}
	if _, _, err := universalDeserializer.Decode(body, nil, &admissionReviewRequest); err != nil {
		http.Error(response, "Bad Request", http.StatusBadRequest)
		log.Error("Unable to deserialize request")
		return
	}

	admissionReviewResponse := admission.AdmissionReview{
		TypeMeta: admissionReviewRequest.TypeMeta,
		Response: &admission.AdmissionResponse{
			UID: admissionReviewRequest.Request.UID,
		},
	}

	patchOperations, err := mutateConfig.mutate(admissionReviewRequest.Request)

	if err != nil {
		// Always allow pods to be created without the agent injected
		admissionReviewResponse.Response.Allowed = true
		admissionReviewResponse.Response.Result = &metav1.Status{
			Message: err.Error(),
		}
	} else {
		patchBytes, err := json.Marshal(patchOperations)
		if err != nil {
			log.Error("Could not marshal JSON patch: ", err)
			http.Error(response, "could not marshal JSON patch", http.StatusInternalServerError)
		}
		admissionReviewResponse.Response.Allowed = true
		admissionReviewResponse.Response.Patch = patchBytes
		admissionReviewResponse.Response.PatchType = new(admission.PatchType)
		*admissionReviewResponse.Response.PatchType = admission.PatchTypeJSONPatch
	}
	data, err := json.Marshal(admissionReviewResponse)
	if err != nil {
		http.Error(response, "Error marshalling response", http.StatusBadRequest)

		return
	}
	if _, err := response.Write(data); err != nil {
		log.Error("Could not write response: ", err)
	}
}

func (mutateConfig *MutateConfig) mutate(request *admission.AdmissionRequest) ([]patchOperation, error) {
	if request.Resource != podResource {
		log.Infof("expect resource to be %v, but got %v", podResource, request.Resource)

		return nil, nil
	}

	raw := request.Object.Raw
	pod := corev1.Pod{}

	if _, _, err := universalDeserializer.Decode(raw, nil, &pod); err != nil {
		return nil, fmt.Errorf("could not deserialize pod object: %v", err)
	}

	if len(pod.Spec.Containers) == 0 {
		log.Warn("No containers found in pod")
		return nil, fmt.Errorf("No containers defined in the Pod")
	}

	if ok, err := mutationRequired(pod.Annotations); !ok {
		return nil, err
	}

	agentPatch := AgentPatch{
		pod:        pod,
		secretName: mutateConfig.SecretName,
	}

	patches, err := agentPatch.GenerateAgentPatches()
	if err != nil {
		return nil, err
	}

	return patches, nil
}

func mutationRequired(annotations map[string]string) (bool, error) {
	var required bool
	switch strings.ToLower(annotations[injectorEnabledAnnotation]) {
	default:
		log.Infof("Skipping mutation: %v annotation not set to enabled or true", injectorEnabledAnnotation)
		required = false
		return required, fmt.Errorf("Skipping mutation: %v annotation not set to enabled or true", injectorEnabledAnnotation)
	case "true", "enabled":
		required = true
	}

	return required, nil
}
