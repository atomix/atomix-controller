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
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func getDefaultReadinessProbe() *corev1.Probe {
	var defaultReadinessProbe = &corev1.Probe{
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{"stat", "/tmp/atomix-ready"},
			},
		},
		InitialDelaySeconds: 5,
		TimeoutSeconds:      10,
		FailureThreshold:    12,
	}

	return defaultReadinessProbe

}

func getDefaultLivenessProbe(cluster *v1beta1.Cluster) *corev1.Probe {
	var probePort int32
	if cluster.Spec.Proxy != nil {
		probePort = 5679
	} else {
		probePort = 5678
	}
	var livenessProbe = &corev1.Probe{
		Handler: corev1.Handler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.IntOrString{Type: intstr.Int, IntVal: probePort},
			},
		},
		InitialDelaySeconds: 60,
		TimeoutSeconds:      10,
	}
	return livenessProbe

}
