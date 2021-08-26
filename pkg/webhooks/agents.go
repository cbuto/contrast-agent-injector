package webhooks

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const (
	javaLanguage               = `java`
	injectorVersionAnnotation  = `contrast-agent-injector/version`
	injectorLanguageAnnotation = `contrast-agent-injector/language`
	injectorConfigAnnotation   = `contrast-agent-injector/config`
)

type Agent interface {
	GeneratePatches() []patchOperation
}

type AgentPatch struct {
	pod        corev1.Pod
	secretName string
}

type AgentAnnotations struct {
	version      *string
	language     *string
	envVarConfig []corev1.EnvVar
}

type JavaAgentConfig struct {
	version        *string
	secretName     *string
	envVarConfig   []corev1.EnvVar
	initContainers []corev1.Container
	volumes        []corev1.Volume
	containers     []corev1.Container
}

func (agentPatch AgentPatch) GenerateAgentPatches() ([]patchOperation, error) {
	var agentAnnotations AgentAnnotations
	err := parseValuesFromAnnotations(agentPatch.pod.Annotations, &agentAnnotations)
	if err != nil {
		return nil, err
	}
	var patches []patchOperation
	var agent Agent
	switch strings.ToLower(*agentAnnotations.language) {
	case javaLanguage:
		agent = JavaAgentConfig{
			version:        agentAnnotations.version,
			initContainers: agentPatch.pod.Spec.InitContainers,
			volumes:        agentPatch.pod.Spec.Volumes,
			containers:     agentPatch.pod.Spec.Containers,
			secretName:     &agentPatch.secretName,
			envVarConfig:   agentAnnotations.envVarConfig,
		}
		patches = agent.GeneratePatches()
	default:
		return nil, fmt.Errorf("Language %v not supported", *agentAnnotations.language)
	}

	return patches, nil
}

func (config JavaAgentConfig) GeneratePatches() []patchOperation {
	var patches []patchOperation

	// TODO: Need to figure out which container to choose (maybe the first is just a limitation to document)
	containerToInject := config.containers[0]

	// TODO: Make this a configmap
	// TODO: Use image that is pre built with the agent in it
	// TODO: Add resources
	initContainerDefinition := []corev1.Container{
		{
			Name:    "contrast-agent-injector",
			Image:   "busybox:1.34.0",
			Command: []string{"/bin/sh", "-c"},
			Args: []string{
				fmt.Sprintf(`echo downloading Contrast agent;
				DOWNLOAD_URL_AGENT_JAVA="https://repository.sonatype.org/service/local/artifact/maven/redirect?r=central-proxy&g=com.contrastsecurity&a=contrast-agent&v=%v"
				wget -q -O /opt/contrast/contrast.jar $DOWNLOAD_URL_AGENT_JAVA;
				echo finished downloading Contrast agent;`, strings.ToUpper(*config.version)),
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "contrast-agent-injector",
					MountPath: "/opt/contrast",
				},
			},
		},
	}

	volumeDefinition := []corev1.Volume{
		{
			Name: "contrast-agent-injector",
		},
		{
			Name: "contrast-agent-injector-yaml",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: *config.secretName,
				},
			},
		},
	}

	volumeMountDefinition := []corev1.VolumeMount{
		{
			Name:      "contrast-agent-injector",
			MountPath: "/opt/contrast",
		},
		{
			Name:      "contrast-agent-injector-yaml",
			MountPath: "/opt/contrast/contrast_security.yaml",
			SubPath:   "contrast_security.yaml",
		},
	}

	envVarDefinitions := []corev1.EnvVar{
		{
			Name:  "JAVA_TOOL_OPTIONS",
			Value: "-javaagent:/opt/contrast/contrast.jar",
		},
		{
			Name:  "CONTRAST_CONFIG_PATH",
			Value: "/opt/contrast/contrast_security.yaml",
		},
		{
			Name:  "CONTRAST__AGENT__JAVA__STANDALONE_APP_NAME",
			Value: containerToInject.Name,
		},
	}

	envVarDefinitions = append(envVarDefinitions, config.envVarConfig...)

	log.Info("Generating patches for agent configuration")
	patches = append(patches, addVolumes(config.volumes, volumeDefinition, "/spec/volumes")...)
	patches = append(patches, addInitContainer(config.initContainers, initContainerDefinition, "/spec/initContainers")...)
	patches = append(patches, addVolumeMounts(containerToInject.VolumeMounts, volumeMountDefinition, "/spec/containers/0/volumeMounts")...)
	patches = append(patches, addEnvVars(containerToInject.Env, envVarDefinitions, "/spec/containers/0/env")...)

	return patches
}

func parseValuesFromAnnotations(annotations map[string]string, agentConfig *AgentAnnotations) error {
	language, languageAnnotationExists := annotations[injectorLanguageAnnotation]
	version, versionAnnotationExists := annotations[injectorVersionAnnotation]
	config, configAnnotationExists := annotations[injectorConfigAnnotation]
	if !languageAnnotationExists && !versionAnnotationExists {
		log.Info("Language and version labels must be set")

		return fmt.Errorf("both %v or %v need to be set", injectorLanguageAnnotation, injectorVersionAnnotation)
	}

	agentConfig.language = &language
	agentConfig.version = &version

	if agentConfig.language == nil || agentConfig.version == nil {
		return fmt.Errorf("both %v or %v need to be set", injectorLanguageAnnotation, injectorVersionAnnotation)
	}

	if configAnnotationExists {
		err := parseConfigAnnotation(config, &agentConfig.envVarConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func parseConfigAnnotation(config string, value *[]corev1.EnvVar) error {
	kvPairs := splitCommaSeparatedString(config)
	// envVars := []corev1.EnvVar{}
	var envVars []corev1.EnvVar
	for _, kvPair := range kvPairs {
		parts := strings.SplitN(kvPair, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("failed to parse stringMap annotation, %v: %v", injectorConfigAnnotation, config)
		}
		key := parts[0]
		value := parts[1]
		if len(key) == 0 {
			return fmt.Errorf("failed to parse stringMap annotation, %v: %v", injectorConfigAnnotation, config)
		}
		envVar := []corev1.EnvVar{
			{
				Name:  key,
				Value: value,
			},
		}
		envVars = append(envVars, envVar...)
	}
	if value != nil {
		*value = envVars
	}

	return nil
}

func splitCommaSeparatedString(commaSeparatedString string) []string {
	var result []string
	parts := strings.Split(commaSeparatedString, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		result = append(result, part)
	}
	return result
}

func addInitContainer(existingInitContainers []corev1.Container, initContainersToAdd []corev1.Container, basePath string) (patch []patchOperation) {
	firstInitContainer := len(existingInitContainers) == 0
	var value interface{}
	for _, container := range initContainersToAdd {
		var existingIndex *int
		op := "add"
		for index, existingInitContainer := range existingInitContainers {
			if container.Name == existingInitContainer.Name {
				log.Infof("Found existing init container %v at index %v", container.Name, index)
				existingIndex = &index
				break
			}
		}
		value = container
		path := basePath
		if existingIndex != nil && !firstInitContainer {
			path = fmt.Sprintf("%v/%v", path, *existingIndex)
			log.Infof("setting path to %v", path)
			op = "replace"
		} else if firstInitContainer {
			firstInitContainer = false
			value = []corev1.Container{container}
		} else {
			path = path + "/-"
		}
		patch = append(patch, patchOperation{
			Op:    op,
			Path:  path,
			Value: value,
		})
	}
	return patch
}

func addVolumes(existingVolumes, volumesToAdd []corev1.Volume, basePath string) (patch []patchOperation) {
	firstVolume := len(existingVolumes) == 0
	var value interface{}
	for _, volume := range volumesToAdd {
		var existingIndex *int
		op := "add"
		for index, existingVolume := range existingVolumes {
			if volume.Name == existingVolume.Name {
				log.Infof("Found existing volume %v at index %v", volume.Name, index)
				existingIndex = &index
				break
			}
		}
		value = volume
		path := basePath
		if existingIndex != nil && !firstVolume {
			path = fmt.Sprintf("%v/%v", path, *existingIndex)
			log.Infof("setting path to %v", path)
			op = "replace"
		} else if firstVolume {
			firstVolume = false
			value = []corev1.Volume{volume}
		} else {
			path = path + "/-"
		}
		patch = append(patch, patchOperation{
			Op:    op,
			Path:  path,
			Value: value,
		})
	}
	return patch
}

func addVolumeMounts(existingVolumeMounts, volumesMountsToAdd []corev1.VolumeMount, basePath string) (patch []patchOperation) {
	firstVolumeMount := len(existingVolumeMounts) == 0
	var value interface{}
	for _, volumeMount := range volumesMountsToAdd {
		var existingIndex *int
		op := "add"
		for index, existingVolumeMount := range existingVolumeMounts {
			if volumeMount.MountPath == existingVolumeMount.MountPath {
				log.Infof("Found existing volume mount %v at index %v", volumeMount.MountPath, index)
				existingIndex = &index
				break
			}
		}
		value = volumeMount
		path := basePath
		if existingIndex != nil && !firstVolumeMount {
			path = fmt.Sprintf("%v/%v", path, *existingIndex)
			log.Infof("setting path to %v", path)
			op = "replace"
		} else if firstVolumeMount {
			firstVolumeMount = false
			value = []corev1.VolumeMount{volumeMount}
		} else {
			path = path + "/-"
		}
		patch = append(patch, patchOperation{
			Op:    op,
			Path:  path,
			Value: value,
		})
	}
	return patch
}

func addEnvVars(existingEnvVars, envVarsToAdd []corev1.EnvVar, basePath string) (patch []patchOperation) {
	firstEnvVar := len(existingEnvVars) == 0
	var value interface{}
	for _, envVar := range envVarsToAdd {
		var existingIndex *int
		op := "add"
		for index, existingEnvVar := range existingEnvVars {
			if envVar.Name == existingEnvVar.Name {
				log.Infof("Found existing env var %v at index %v", envVar.Name, index)
				existingIndex = &index
				break
			}
		}
		value = envVar
		path := basePath
		if existingIndex != nil && !firstEnvVar {
			path = fmt.Sprintf("%v/%v", path, *existingIndex)
			log.Infof("setting path to %v", path)
			op = "replace"
		} else if firstEnvVar {
			firstEnvVar = false
			value = []corev1.EnvVar{envVar}
		} else {
			path = path + "/-"
		}
		patch = append(patch, patchOperation{
			Op:    op,
			Path:  path,
			Value: value,
		})
	}
	return patch
}
