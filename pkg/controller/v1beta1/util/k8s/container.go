// Copyright 2020-present Open Networking Foundation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s

import (
	corev1 "k8s.io/api/core/v1"
)

// NewContainer creates a new container builder
func NewContainer() ContainerBuilder {
	return &Container{}
}

// ContainerBuilder container builder interface
type ContainerBuilder interface {
	SetName(string) ContainerBuilder
	SetImage(string) ContainerBuilder
	SetArgs(...string) ContainerBuilder
	SetEnv(env []corev1.EnvVar) ContainerBuilder
	SetCommand(...string) ContainerBuilder
	SetPullPolicy(policy corev1.PullPolicy) ContainerBuilder
	SetReadinessProbe(probe *corev1.Probe) ContainerBuilder
	SetLivenessProbe(probe *corev1.Probe) ContainerBuilder
	SetVolumeMounts([]corev1.VolumeMount) ContainerBuilder
	SetResources(corev1.ResourceRequirements) ContainerBuilder
	SetPorts([]corev1.ContainerPort) ContainerBuilder
	Build() Container
}

// Container container type defines an abstraction for containers
type Container struct {
	name           string
	image          string
	command        []string
	args           []string
	env            []corev1.EnvVar
	resources      corev1.ResourceRequirements
	pullPolicy     corev1.PullPolicy
	readinessProbe *corev1.Probe
	livenessProbe  *corev1.Probe
	volumeMounts   []corev1.VolumeMount
	ports          []corev1.ContainerPort
}

// SetResources sets container resources
func (c *Container) SetResources(resources corev1.ResourceRequirements) ContainerBuilder {
	c.resources = resources
	return c
}

// Resources returns container resources
func (c *Container) Resources() corev1.ResourceRequirements {
	return c.resources
}

// SetVolumeMounts sets mounted volumes
func (c *Container) SetVolumeMounts(volumeMounts []corev1.VolumeMount) ContainerBuilder {
	c.volumeMounts = volumeMounts
	return c
}

// VolumeMounts returns container mounted volumes
func (c *Container) VolumeMounts() []corev1.VolumeMount {
	return c.volumeMounts
}

// ReadinessProbe returns readinessProbe info
func (c *Container) ReadinessProbe() *corev1.Probe {
	return c.readinessProbe
}

// SetReadinessProbe sets container readinessProbe
func (c *Container) SetReadinessProbe(probe *corev1.Probe) ContainerBuilder {
	c.readinessProbe = probe
	return c
}

// LivenessProbe returns livenessprobe info
func (c *Container) LivenessProbe() *corev1.Probe {
	return c.livenessProbe
}

// SetLivenessProbe sets container livenessProbe
func (c *Container) SetLivenessProbe(probe *corev1.Probe) ContainerBuilder {
	c.livenessProbe = probe
	return c
}

// Env returns container env variables
func (c *Container) Env() []corev1.EnvVar {
	return c.env

}

// SetEnv sets environment variables
func (c *Container) SetEnv(env []corev1.EnvVar) ContainerBuilder {
	c.env = env
	return c
}

// Ports return a set of container ports
func (c *Container) Ports() []corev1.ContainerPort {
	return c.ports
}

// SetPorts sets a container ports
func (c *Container) SetPorts(ports []corev1.ContainerPort) ContainerBuilder {
	c.ports = ports
	return c
}

// PullPolicy returns the pull policy
func (c *Container) PullPolicy() corev1.PullPolicy {
	return c.pullPolicy
}

// SetPullPolicy sets pull policy for a container
func (c *Container) SetPullPolicy(pullPolicy corev1.PullPolicy) ContainerBuilder {
	c.pullPolicy = pullPolicy
	return c
}

// Command returns a container command
func (c *Container) Command() []string {
	return c.command
}

// SetCommand sets a container command
func (c *Container) SetCommand(command ...string) ContainerBuilder {
	c.command = command
	return c
}

// Args returns a container args
func (c *Container) Args() []string {
	return c.args
}

// SetArgs sets a container arguments
func (c *Container) SetArgs(args ...string) ContainerBuilder {
	c.args = args
	return c
}

// SetName sets a container name
func (c *Container) SetName(name string) ContainerBuilder {
	c.name = name
	return c
}

// Name returns the name of a container
func (c *Container) Name() string {
	return c.name
}

// SetImage sets a container image
func (c *Container) SetImage(image string) ContainerBuilder {
	c.image = image
	return c
}

// Image returns the name of a container image
func (c *Container) Image() string {
	return c.image
}

// Build builds a container
func (c *Container) Build() Container {
	return Container{
		name:           c.name,
		image:          c.image,
		command:        c.command,
		args:           c.args,
		resources:      c.resources,
		livenessProbe:  c.livenessProbe,
		pullPolicy:     c.pullPolicy,
		readinessProbe: c.readinessProbe,
		ports:          c.ports,
		env:            c.env,
		volumeMounts:   c.volumeMounts,
	}
}
