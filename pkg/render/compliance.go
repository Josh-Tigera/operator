// Copyright (c) 2019 Tigera, Inc. All rights reserved.

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

package render

import (
	"fmt"

	v3 "github.com/tigera/api/pkg/apis/projectcalico/v3"

	ocsv1 "github.com/openshift/api/security/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	ComplianceNamespace = "tigera-compliance"
)

const (
	ElasticsearchUserComplianceBenchmarker = "tigera-ee-compliance-benchmarker"
	ElasticsearchUserComplianceController  = "tigera-ee-compliance-controller"
	ElasticsearchUserComplianceReporter    = "tigera-ee-compliance-reporter"
	ElasticsearchUserComplianceSnapshotter = "tigera-ee-compliance-snapshotter"
	ElasticsearchUserComplianceServer      = "tigera-ee-compliance-server"
	ElasticsearchUserCurator               = "tigera-ee-curator"
)

func Compliance(
	esSecrets []*corev1.Secret,
	registry string,
	clusterName string,
	pullSecrets []*corev1.Secret,
	openshift bool,
) Component {
	return &complianceComponent{
		esSecrets:   esSecrets,
		registry:    registry,
		clusterName: clusterName,
		pullSecrets: pullSecrets,
		openshift:   openshift,
	}
}

type complianceComponent struct {
	esSecrets   []*corev1.Secret
	registry    string
	clusterName string
	pullSecrets []*corev1.Secret
	openshift   bool
}

func (c *complianceComponent) Objects() []runtime.Object {
	complianceObjs := append(
		[]runtime.Object{createNamespace(ComplianceNamespace, c.openshift)},
		copyImagePullSecrets(c.pullSecrets, ComplianceNamespace)...,
	)
	complianceObjs = append(complianceObjs,
		c.complianceControllerServiceAccount(),
		c.complianceControllerRole(),
		c.complianceControllerClusterRole(),
		c.complianceControllerRoleBinding(),
		c.complianceControllerClusterRoleBinding(),
		c.complianceControllerDeployment(),

		c.complianceReporterServiceAccount(),
		c.complianceReporterClusterRole(),
		c.complianceReporterClusterRoleBinding(),
		c.complianceReporterPodTemplate(),

		c.complianceServerServiceAccount(),
		c.complianceServerClusterRole(),
		c.complianceServerClusterRoleBinding(),
		c.complianceServerService(),
		c.complianceServerDeployment(),

		c.complianceSnapshotterServiceAccount(),
		c.complianceSnapshotterClusterRole(),
		c.complianceSnapshotterClusterRoleBinding(),
		c.complianceSnapshotterDeployment(),

		c.complianceBenchmarkerServiceAccount(),
		c.complianceBenchmarkerClusterRole(),
		c.complianceBenchmarkerClusterRoleBinding(),
		c.complianceBenchmarkerDaemonSet(),

		c.complianceGlobalReportInventory(),
		c.complianceGlobalReportNetworkAccess(),
		c.complianceGlobalReportPolicyAudit(),
		c.complianceGlobalReportCISBenchmark(),
	)

	if c.openshift {
		complianceObjs = append(complianceObjs, c.complianceBenchmarkerSecurityContextConstraints())
	}

	complianceObjs = append(complianceObjs, copySecrets(ComplianceNamespace, c.esSecrets...)...)

	return complianceObjs
}

func (c *complianceComponent) Ready() bool {
	return true
}

var complianceBoolTrue = true
var complianceReplicas int32 = 1

// complianceLivenssProbe is the liveness probe to use for compliance components.
// They all use the same liveness configuration, so we just define it once here.
var complianceLivenessProbe = &corev1.Probe{
	Handler: corev1.Handler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: "/liveness",
			Port: intstr.FromInt(9099),
		},
	},
}

func (c *complianceComponent) complianceControllerServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta:   metav1.TypeMeta{Kind: "ServiceAccount", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-controller", Namespace: ComplianceNamespace},
	}
}

func (c *complianceComponent) complianceControllerRole() *rbacv1.Role {
	return &rbacv1.Role{
		TypeMeta:   metav1.TypeMeta{Kind: "Role", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-controller", Namespace: ComplianceNamespace},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"batch"},
				Resources: []string{"jobs"},
				Verbs:     []string{"create", "list", "get", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"podtemplates"},
				Verbs:     []string{"get"},
			},
		},
	}
}

func (c *complianceComponent) complianceControllerClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRole", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-controller"},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"projectcalico.org"},
				Resources: []string{"globalreports"},
				Verbs:     []string{"list"},
			},
			{
				APIGroups: []string{"projectcalico.org"},
				Resources: []string{"globalreports/status"},
				Verbs:     []string{"update", "list"},
			},
			{
				APIGroups: []string{"projectcalico.org"},
				Resources: []string{"globalreports/finalizers"},
				Verbs:     []string{"update"},
			},
		},
	}
}

func (c *complianceComponent) complianceControllerRoleBinding() *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		TypeMeta:   metav1.TypeMeta{Kind: "RoleBinding", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-controller", Namespace: ComplianceNamespace},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "tigera-compliance-controller",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "tigera-compliance-controller",
				Namespace: ComplianceNamespace,
			},
		},
	}
}

func (c *complianceComponent) complianceControllerClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRoleBinding", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-controller"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "tigera-compliance-controller",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "tigera-compliance-controller",
				Namespace: ComplianceNamespace,
			},
		},
	}
}

func (c *complianceComponent) complianceControllerDeployment() *appsv1.Deployment {
	envVars := []corev1.EnvVar{
		{Name: "LOG_LEVEL", Value: "info"},
		{Name: "TIGERA_COMPLIANCE_JOB_NAMESPACE", Value: ComplianceNamespace},
		{Name: "TIGERA_COMPLIANCE_MAX_FAILED_JOBS_HISTORY", Value: "3"},
		{Name: "TIGERA_COMPLIANCE_MAX_JOB_RETRIES", Value: "6"},
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "compliance-controller",
			Namespace: ComplianceNamespace,
			Labels: map[string]string{
				"k8s-app": "compliance-controller",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &complianceReplicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "compliance-controller"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "compliance-controller",
					Namespace: ComplianceNamespace,
					Labels: map[string]string{
						"k8s-app": "compliance-controller",
					},
				},
				Spec: ElasticsearchPodSpecDecorate(corev1.PodSpec{
					NodeSelector: map[string]string{
						"beta.kubernetes.io/os": "linux",
					},
					ServiceAccountName: "tigera-compliance-controller",
					Tolerations: []corev1.Toleration{
						{
							Key:    "node-role.kubernetes.io/master",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					ImagePullSecrets: getImagePullSecretReferenceList(c.pullSecrets),
					Containers: []corev1.Container{
						ElasticsearchContainerDecorate(corev1.Container{
							Name:            "compliance-controller",
							Image:           constructImage(ComplianceControllerImage, c.registry),
							Env:             envVars,
							LivenessProbe:   complianceLivenessProbe,
						}, c.clusterName, ElasticsearchUserComplianceController),
					},
				}),
			},
		},
	}
}

func (c *complianceComponent) complianceReporterServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta:   metav1.TypeMeta{Kind: "ServiceAccount", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-reporter", Namespace: ComplianceNamespace},
	}
}

func (c *complianceComponent) complianceReporterClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRole", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-reporter"},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"projectcalico.org"},
				Resources: []string{"globalreporttypes", "globalreports"},
				Verbs:     []string{"get"},
			},
		},
	}
}

func (c *complianceComponent) complianceReporterClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRoleBinding", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-reporter"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "tigera-compliance-reporter",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "tigera-compliance-reporter",
				Namespace: ComplianceNamespace,
			},
		},
	}
}

func (c *complianceComponent) complianceReporterPodTemplate() *corev1.PodTemplate {
	dirOrCreate := corev1.HostPathDirectoryOrCreate
	privileged := true

	envVars := []corev1.EnvVar{
		{Name: "LOG_LEVEL", Value: "warning"},
		{Name: "TIGERA_COMPLIANCE_JOB_NAMESPACE", Value: ComplianceNamespace},
	}
	return &corev1.PodTemplate{
		TypeMeta: metav1.TypeMeta{Kind: "PodTemplate", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tigera.io.report",
			Namespace: ComplianceNamespace,
			Labels: map[string]string{
				"k8s-app": "compliance-reporter",
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tigera.io.report",
				Namespace: ComplianceNamespace,
				Labels: map[string]string{
					"k8s-app": "compliance-reporter",
				},
			},
			Spec: ElasticsearchPodSpecDecorate(corev1.PodSpec{
				NodeSelector:       map[string]string{"beta.kubernetes.io/os": "linux"},
				ServiceAccountName: "tigera-compliance-reporter",
				Tolerations: []corev1.Toleration{
					{
						Key:    "node-role.kubernetes.io/master",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
				ImagePullSecrets: getImagePullSecretReferenceList(c.pullSecrets),
				Containers: []corev1.Container{
					ElasticsearchContainerDecorate(corev1.Container{
						Name:          "reporter",
						Image:         constructImage(ComplianceReporterImage, c.registry),
						Env:           envVars,
						LivenessProbe: complianceLivenessProbe,
						SecurityContext: &corev1.SecurityContext{
							Privileged: &privileged,
						},
						VolumeMounts: []corev1.VolumeMount{
							{MountPath: "/var/log/calico", Name: "var-log-calico"},
						},
					}, c.clusterName, ElasticsearchUserComplianceReporter),
				},
				Volumes: []corev1.Volume{
					{
						Name: "var-log-calico",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/var/log/calico",
								Type: &dirOrCreate,
							},
						},
					},
				},
			}),
		},
	}
}

func (c *complianceComponent) complianceServerServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta:   metav1.TypeMeta{Kind: "ServiceAccount", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-server", Namespace: ComplianceNamespace},
	}
}

func (c *complianceComponent) complianceServerClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRole", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-server"},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"projectcalico.org"},
				Resources: []string{"globalreporttypes", "globalreports"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"authentication.k8s.io"},
				Resources: []string{"tokenreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"subjectaccessreviews"},
				Verbs:     []string{"create"},
			},
		},
	}
}

func (c *complianceComponent) complianceServerClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRoleBinding", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-server"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "tigera-compliance-server",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "tigera-compliance-server",
				Namespace: ComplianceNamespace,
			},
		},
	}
}

func (c *complianceComponent) complianceServerService() *corev1.Service {
	return &corev1.Service{
		TypeMeta:   metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "compliance", Namespace: ComplianceNamespace},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "compliance-api",
					Port:       443,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(5443),
				},
			},
			Selector: map[string]string{"k8s-app": "compliance-server"},
		},
	}
}

func (c *complianceComponent) complianceServerDeployment() *appsv1.Deployment {
	envVars := []corev1.EnvVar{
		{Name: "LOG_LEVEL", Value: "info"},
		{Name: "TIGERA_COMPLIANCE_JOB_NAMESPACE", Value: ComplianceNamespace},
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "compliance-server",
			Namespace: ComplianceNamespace,
			Labels: map[string]string{
				"k8s-app": "compliance-server",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &complianceReplicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "compliance-server"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "compliance-server",
					Namespace: ComplianceNamespace,
					Labels: map[string]string{
						"k8s-app": "compliance-server",
					},
				},
				Spec: ElasticsearchPodSpecDecorate(corev1.PodSpec{
					NodeSelector:       map[string]string{"beta.kubernetes.io/os": "linux"},
					ServiceAccountName: "tigera-compliance-server",
					Tolerations: []corev1.Toleration{
						{
							Key:    "node-role.kubernetes.io/master",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					ImagePullSecrets: getImagePullSecretReferenceList(c.pullSecrets),
					Containers: []corev1.Container{
						ElasticsearchContainerDecorate(corev1.Container{
							Name:            "compliance-server",
							Image:           constructImage(ComplianceServerImage, c.registry),
							Env:             envVars,
						}, c.clusterName, ElasticsearchUserComplianceServer),
					},
				}),
			},
		},
	}
}

func (c *complianceComponent) complianceSnapshotterServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta:   metav1.TypeMeta{Kind: "ServiceAccount", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-snapshotter", Namespace: ComplianceNamespace},
	}
}

func (c *complianceComponent) complianceSnapshotterClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRole", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-snapshotter"},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"networking.k8s.io", "authentication.k8s.io", ""},
				Resources: []string{"networkpolicies", "nodes", "namespaces", "pods", "serviceaccounts",
					"endpoints", "services"},
				Verbs: []string{"get", "list"},
			},
			{
				APIGroups: []string{"projectcalico.org"},
				Resources: []string{"globalnetworkpolicies", "tier.globalnetworkpolicies",
					"stagedglobalnetworkpolicies", "tier.stagedglobalnetworkpolicies",
					"networkpolicies", "tier.networkpolicies",
					"stagednetworkpolicies", "tier.stagednetworkpolicies",
					"stagedkubernetesnetworkpolicies",
					"tiers", "hostendpoints",
					"globalnetworksets", "networksets"},
				Verbs: []string{"get", "list"},
			},
		},
	}
}

func (c *complianceComponent) complianceSnapshotterClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRoleBinding", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-snapshotter"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "tigera-compliance-snapshotter",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "tigera-compliance-snapshotter",
				Namespace: ComplianceNamespace,
			},
		},
	}
}

func (c *complianceComponent) complianceSnapshotterDeployment() *appsv1.Deployment {
	envVars := []corev1.EnvVar{
		{Name: "LOG_LEVEL", Value: "info"},
		{Name: "TIGERA_COMPLIANCE_JOB_NAMESPACE", Value: ComplianceNamespace},
		{Name: "TIGERA_COMPLIANCE_MAX_FAILED_JOBS_HISTORY", Value: "3"},
		{Name: "TIGERA_COMPLIANCE_SNAPSHOT_HOUR", Value: "0"},
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "compliance-snapshotter",
			Namespace: ComplianceNamespace,
			Labels: map[string]string{
				"k8s-app": "compliance-snapshotter",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &complianceReplicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "compliance-snapshotter"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "compliance-snapshotter",
					Namespace: ComplianceNamespace,
					Labels: map[string]string{
						"k8s-app": "compliance-snapshotter",
					},
				},
				Spec: ElasticsearchPodSpecDecorate(corev1.PodSpec{
					NodeSelector:       map[string]string{"beta.kubernetes.io/os": "linux"},
					ServiceAccountName: "tigera-compliance-snapshotter",
					Tolerations: []corev1.Toleration{
						{
							Key:    "node-role.kubernetes.io/master",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
					ImagePullSecrets: getImagePullSecretReferenceList(c.pullSecrets),
					Containers: []corev1.Container{
						ElasticsearchContainerDecorate(corev1.Container{
							Name:            "compliance-snapshotter",
							Image:           constructImage(ComplianceSnapshotterImage, c.registry),
							Env:             envVars,
							LivenessProbe:   complianceLivenessProbe,
						}, c.clusterName, ElasticsearchUserComplianceSnapshotter),
					},
				}),
			},
		},
	}
}

func (c *complianceComponent) complianceBenchmarkerServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta:   metav1.TypeMeta{Kind: "ServiceAccount", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-benchmarker", Namespace: ComplianceNamespace},
	}
}

func (c *complianceComponent) complianceBenchmarkerClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRole", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-benchmarker"},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get"},
			},
		},
	}
}

func (c *complianceComponent) complianceBenchmarkerClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRoleBinding", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "tigera-compliance-benchmarker"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "tigera-compliance-benchmarker",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "tigera-compliance-benchmarker",
				Namespace: ComplianceNamespace,
			},
		},
	}
}

func (c *complianceComponent) complianceBenchmarkerDaemonSet() *appsv1.DaemonSet {
	envVars := []corev1.EnvVar{
		{Name: "LOG_LEVEL", Value: "info"},
		{Name: "NODENAME", ValueFrom: &corev1.EnvVarSource{ FieldRef: &corev1.ObjectFieldSelector { FieldPath: "spec.nodeName" } }},
	}

	volMounts := []corev1.VolumeMount{
		{Name: "var-lib-etcd", MountPath: "/var/lib/etcd", ReadOnly: true},
		{Name: "var-lib-kubelet", MountPath: "/var/lib/kubelet", ReadOnly: true},
		{Name: "etc-systemd", MountPath: "/etc/systemd", ReadOnly: true},
		{Name: "etc-kubernetes", MountPath: "/etc/kubernetes", ReadOnly: true},
		{Name: "usr-bin", MountPath: "/usr/bin", ReadOnly: true},
	}

	vols := []corev1.Volume{
		{
			Name:         "var-lib-etcd",
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/etcd"}},
		},
		{
			Name:         "var-lib-kubelet",
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/kubelet"}},
		},
		{
			Name:         "etc-systemd",
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/etc/systemd"}},
		},
		{
			Name:         "etc-kubernetes",
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/etc/kubernetes"}},
		},
		{
			Name:         "usr-bin",
			VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/usr/bin"}},
		},
	}

	return &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{Kind: "DaemonSet", APIVersion: "apps/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "compliance-benchmarker",
			Namespace: ComplianceNamespace,
			Labels:    map[string]string{"k8s-app": "compliance-benchmarker"},
		},

		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "compliance-benchmarker"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "compliance-benchmarker",
					Namespace: ComplianceNamespace,
					Labels: map[string]string{
						"k8s-app": "compliance-benchmarker",
					},
				},
				Spec: ElasticsearchPodSpecDecorate(corev1.PodSpec{
					NodeSelector:       map[string]string{"beta.kubernetes.io/os": "linux"},
					ServiceAccountName: "tigera-compliance-benchmarker",
					HostPID:            true,
					Tolerations: []corev1.Toleration{
						{
							Effect:   corev1.TaintEffectNoSchedule,
							Operator: corev1.TolerationOpExists,
						},
						{
							Key:      "CriticalAddonsOnly",
							Operator: corev1.TolerationOpExists,
						},
						{
							Effect:   corev1.TaintEffectNoExecute,
							Operator: corev1.TolerationOpExists,
						},
					},
					ImagePullSecrets: getImagePullSecretReferenceList(c.pullSecrets),
					Containers: []corev1.Container{
						ElasticsearchContainerDecorate(corev1.Container{
							Name:          "compliance-benchmarker",
							Image:         constructImage(ComplianceBenchmarkerImage, c.registry),
							Env:           envVars,
							VolumeMounts:  volMounts,
							LivenessProbe: complianceLivenessProbe,
						}, c.clusterName, ElasticsearchUserComplianceBenchmarker),
					},
					Volumes: vols,
				}),
			},
		},
	}
}

func (c *complianceComponent) complianceBenchmarkerSecurityContextConstraints() *ocsv1.SecurityContextConstraints {
	return &ocsv1.SecurityContextConstraints{
		TypeMeta:                 metav1.TypeMeta{Kind: "SecurityContextConstraints", APIVersion: "security.openshift.io/v1"},
		ObjectMeta:               metav1.ObjectMeta{Name: "tigera-compliance-benchmarker"},
		AllowHostDirVolumePlugin: true,
		AllowHostIPC:             false,
		AllowHostNetwork:         false,
		AllowHostPID:             true,
		AllowHostPorts:           false,
		AllowPrivilegeEscalation: &complianceBoolTrue,
		AllowPrivilegedContainer: true,
		FSGroup:                  ocsv1.FSGroupStrategyOptions{Type: ocsv1.FSGroupStrategyRunAsAny},
		RunAsUser:                ocsv1.RunAsUserStrategyOptions{Type: ocsv1.RunAsUserStrategyRunAsAny},
		ReadOnlyRootFilesystem:   false,
		SELinuxContext:           ocsv1.SELinuxContextStrategyOptions{Type: ocsv1.SELinuxStrategyMustRunAs},
		SupplementalGroups:       ocsv1.SupplementalGroupsStrategyOptions{Type: ocsv1.SupplementalGroupsStrategyRunAsAny},
		Users: []string{
			fmt.Sprintf("system:serviceaccount:%s:tigera-compliance-benchmarker", ComplianceNamespace),
		},
		Groups:  []string{"system:authenticated"},
		Volumes: []ocsv1.FSType{"*"},
	}
}

func (c *complianceComponent) complianceGlobalReportInventory() *v3.GlobalReportType {
	return &v3.GlobalReportType{
		TypeMeta: metav1.TypeMeta{Kind: "GlobalReportType", APIVersion: "projectcalico.org/v3"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "inventory",
			Labels: map[string]string{
				"global-report-type": "inventory",
			},
		},
		Spec: v3.ReportTypeSpec{
			DownloadTemplates: []v3.ReportTemplate{
				{
					Name: "summary.csv",
					Template: `
      {{ $c := csv }}
      {{- $c := $c.AddColumn "startTime"                     "{{ dateRfc3339 .StartTime }}" }}
      {{- $c := $c.AddColumn "endTime"                       "{{ dateRfc3339 .EndTime }}" }}
      {{- $c := $c.AddColumn "endpointSelector"              "{{ if .ReportSpec.Endpoints }}{{ .ReportSpec.Endpoints.Selector }}{{ end }}" }}
      {{- $c := $c.AddColumn "namespaceNames"                "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.Namespaces }}{{ join \";\" .ReportSpec.Endpoints.Namespaces.Names }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "namespaceSelector"             "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.Namespaces }}{{ .ReportSpec.Endpoints.Namespaces.Selector }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "serviceAccountNames"           "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.ServiceAccounts }}{{ join \";\" .ReportSpec.Endpoints.ServiceAccounts.Names }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "serviceAccountSelectors"       "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.ServiceAccounts }}{{ .ReportSpec.Endpoints.ServiceAccounts.Selector }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "endpointsNumInScope"           "{{ .EndpointsSummary.NumTotal }}" }}
      {{- $c := $c.AddColumn "endpointsNumIngressProtected"  "{{ .EndpointsSummary.NumIngressProtected }}" }}
      {{- $c := $c.AddColumn "endpointsNumEgressProtected"   "{{ .EndpointsSummary.NumEgressProtected }}" }}
      {{- $c := $c.AddColumn "namespacesNumInScope"          "{{ .NamespacesSummary.NumTotal }}" }}
      {{- $c := $c.AddColumn "namespacesNumIngressProtected" "{{ .NamespacesSummary.NumIngressProtected }}" }}
      {{- $c := $c.AddColumn "namespacesNumEgressProtected"  "{{ .NamespacesSummary.NumEgressProtected }}" }}
      {{- $c := $c.AddColumn "serviceAccountsNumInScope"     "{{ .EndpointsSummary.NumServiceAccounts }}" }}
      {{- $c.Render . }}
`,
				},
				{
					Name: "endpoints.csv",
					Template: `
      {{ $c := csv }}
      {{- $c := $c.AddColumn "endpoint"         "{{ .Endpoint }}" }}
      {{- $c := $c.AddColumn "ingressProtected" "{{ .IngressProtected }}" }}
      {{- $c := $c.AddColumn "egressProtected"  "{{ .EgressProtected }}" }}
      {{- $c := $c.AddColumn "envoyEnabled"     "{{ .EnvoyEnabled }}" }}
      {{- $c := $c.AddColumn "appliedPolicies"  "{{ join \";\" .AppliedPolicies }}" }}
      {{- $c := $c.AddColumn "services"         "{{ join \";\" .Services }}" }}
      {{- $c.Render .Endpoints }}
`,
				},
				{
					Name: "namespaces.csv",
					Template: `
      {{ $c := csv }}
      {{- $c := $c.AddColumn "namespace"        "{{ .Namespace }}" }}
      {{- $c := $c.AddColumn "ingressProtected" "{{ .IngressProtected }}" }}
      {{- $c := $c.AddColumn "egressProtected"  "{{ .EgressProtected }}" }}
      {{- $c := $c.AddColumn "envoyEnabled"     "{{ .EnvoyEnabled }}" }}
      {{- $c.Render .Namespaces }}
`,
				},
				{
					Name: "services.csv",
					Template: `
      {{ $c := csv }}
      {{- $c := $c.AddColumn "service"          "{{ .Service }}" }}
      {{- $c := $c.AddColumn "ingressProtected" "{{ .IngressProtected }}" }}
      {{- $c := $c.AddColumn "envoyEnabled"     "{{ .EnvoyEnabled }}" }}
      {{- $c.Render .Services }}
`,
				},
			},
			IncludeEndpointData: true,
			UISummaryTemplate: v3.ReportTemplate{
				Name:     "ui-summary.json",
				Template: `{"heading":"Inscope vs Protected","type":"panel","widgets":[{"data":[{"label":"Protected ingress","value":{{ .EndpointsSummary.NumIngressProtected }}}],"heading":"Endpoints","summary":{"label":"Total","total":{{.EndpointsSummary.NumTotal }}},"type":"radialbarchart"},{"data":[{"label":"Protected ingress","value":{{ .NamespacesSummary.NumIngressProtected }}}],"heading":"Namespaces","summary":{"label":"Total","total":{{.NamespacesSummary.NumTotal }}},"type":"radialbarchart"},{"data":[{"label":"Protected egress","value":{{ .EndpointsSummary.NumEgressProtected }}}],"heading":"Endpoints","summary":{"label":"Total","total":{{.EndpointsSummary.NumTotal }}},"type":"radialbarchart"},{"data":[{"label":"Protected egress","value":{{ .NamespacesSummary.NumEgressProtected }}}],"heading":"Namespaces","summary":{"label":"Total","total":{{.NamespacesSummary.NumTotal }}},"type":"radialbarchart"}]}`,
			},
		},
	}
}

func (c *complianceComponent) complianceGlobalReportNetworkAccess() *v3.GlobalReportType {
	return &v3.GlobalReportType{
		TypeMeta: metav1.TypeMeta{Kind: "GlobalReportType", APIVersion: "projectcalico.org/v3"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "network-access",
			Labels: map[string]string{
				"global-report-type": "network-access",
			},
		},
		Spec: v3.ReportTypeSpec{
			DownloadTemplates: []v3.ReportTemplate{
				{
					Name: "summary.csv",
					Template: `
      {{ $c := csv }}
      {{- $c := $c.AddColumn "startTime"                             "{{ dateRfc3339 .StartTime }}" }}
      {{- $c := $c.AddColumn "endTime"                               "{{ dateRfc3339 .EndTime }}" }}
      {{- $c := $c.AddColumn "endpointSelector"                      "{{ if .ReportSpec.Endpoints }}{{ .ReportSpec.Endpoints.Selector }}{{ end }}" }}
      {{- $c := $c.AddColumn "namespaceNames"                        "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.Namespaces }}{{ join \";\" .ReportSpec.Endpoints.Namespaces.Names }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "namespaceSelector"                     "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.Namespaces }}{{ .ReportSpec.Endpoints.Namespaces.Selector }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "serviceAccountNames"                   "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.ServiceAccounts }}{{ join \";\" .ReportSpec.Endpoints.ServiceAccounts.Names }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "serviceAccountSelectors"               "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.ServiceAccounts }}{{ .ReportSpec.Endpoints.ServiceAccounts.Selector }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "endpointsNumIngressProtected"          "{{ .EndpointsSummary.NumIngressProtected }}" }}
      {{- $c := $c.AddColumn "endpointsNumEgressProtected"           "{{ .EndpointsSummary.NumEgressProtected }}" }}
      {{- $c := $c.AddColumn "endpointsNumIngressUnprotected"        "{{ sub .EndpointsSummary.NumTotal .EndpointsSummary.NumIngressProtected }}" }}
      {{- $c := $c.AddColumn "endpointsNumEgressUnprotected"         "{{ sub .EndpointsSummary.NumTotal .EndpointsSummary.NumEgressProtected  }}" }}
      {{- $c := $c.AddColumn "endpointsNumIngressFromInternet"       "{{ .EndpointsSummary.NumIngressFromInternet }}" }}
      {{- $c := $c.AddColumn "endpointsNumEgressToInternet"          "{{ .EndpointsSummary.NumEgressToInternet }}" }}
      {{- $c := $c.AddColumn "endpointsNumIngressFromOtherNamespace" "{{ .EndpointsSummary.NumIngressFromOtherNamespace }}" }}
      {{- $c := $c.AddColumn "endpointsNumEgressToOtherNamespace"    "{{ .EndpointsSummary.NumEgressToOtherNamespace }}" }}
      {{- $c := $c.AddColumn "endpointsNumEnvoyEnabled"              "{{ .EndpointsSummary.NumEnvoyEnabled }}" }}
      {{- $c.Render . }}
`,
				},
				{
					Name: "endpoints.csv",
					Template: `
      {{ $c := csv }}
      {{- $c := $c.AddColumn "endpoint"                                  "{{ .Endpoint }}" }}
      {{- $c := $c.AddColumn "ingressProtected"                          "{{ .IngressProtected }}" }}
      {{- $c := $c.AddColumn "egressProtected"                           "{{ .EgressProtected }}" }}
      {{- $c := $c.AddColumn "ingressFromInternet"                       "{{ .IngressFromInternet }}" }}
      {{- $c := $c.AddColumn "egressToInternet"                          "{{ .EgressToInternet }}" }}
      {{- $c := $c.AddColumn "ingressFromOtherNamespace"                 "{{ .IngressFromOtherNamespace }}" }}
      {{- $c := $c.AddColumn "egressToOtherNamespace"                    "{{ .EgressToOtherNamespace }}" }}
      {{- $c := $c.AddColumn "envoyEnabled"                              "{{ .EnvoyEnabled }}" }}
      {{- $c := $c.AddColumn "appliedPolicies"                           "{{ join \";\" .AppliedPolicies }}" }}
      {{- $c := $c.AddColumn "trafficAggregationPrefix"                  "{{ flowsPrefix . }}" }}
      {{- $c := $c.AddColumn "endpointsGeneratingTrafficToThisEndpoint"  "{{ join \";\" (flowsIngress .) }}" }}
      {{- $c := $c.AddColumn "endpointsReceivingTrafficFromThisEndpoint" "{{ join \";\" (flowsEgress .) }}" }}
      {{- $c.Render .Endpoints }}
`,
				},
			},
			IncludeEndpointData: true,
			UISummaryTemplate: v3.ReportTemplate{
				Name: "ui-summary.json",
				Template: `
    {"heading":"Inscope vs Protected","type":"panel","widgets":[{"data":[{"label":"Protected ingress","value":{{ .EndpointsSummary.NumIngressProtected }}}],"heading":"Endpoints","summary":{"label":"Total","total":{{.EndpointsSummary.NumTotal }}},"type":"radialbarchart"},{"data":[{"label":"Protected ingress","value":{{ .NamespacesSummary.NumIngressProtected }}}],"heading":"Namespaces","summary":{"label":"Total","total":{{.NamespacesSummary.NumTotal }}},"type":"radialbarchart"},{"data":[{"label":"Protected egress","value":{{ .EndpointsSummary.NumEgressProtected }}}],"heading":"Endpoints","summary":{"label":"Total","total":{{.EndpointsSummary.NumTotal }}},"type":"radialbarchart"},{"data":[{"label":"Protected egress","value":{{ .NamespacesSummary.NumEgressProtected }}}],"heading":"Namespaces","summary":{"label":"Total","total":{{.NamespacesSummary.NumTotal }}},"type":"radialbarchart"}]}
`,
			},
		},
	}
}

func (c *complianceComponent) complianceGlobalReportPolicyAudit() *v3.GlobalReportType {
	return &v3.GlobalReportType{
		TypeMeta: metav1.TypeMeta{Kind: "GlobalReportType", APIVersion: "projectcalico.org/v3"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "policy-audit",
			Labels: map[string]string{
				"global-report-type": "policy-audit",
			},
		},
		Spec: v3.ReportTypeSpec{
			AuditEventsSelection: &v3.AuditEventsSelection{
				Resources: []v3.AuditResource{
					{
						Resource: "globalnetworkpolicies",
					},
					{
						Resource: "networkpolicies",
					},
					{
						Resource: "stagedglobalnetworkpolicies",
					},
					{
						Resource: "stagednetworkpolicies",
					},
					{
						Resource: "stagedkubernetesnetworkpolicies",
					},
				},
			},
			DownloadTemplates: []v3.ReportTemplate{
				{
					Name: "summary.csv",
					Template: `
      {{ $c := csv }}
      {{- $c := $c.AddColumn "startTime"               "{{ dateRfc3339 .StartTime }}" }}
      {{- $c := $c.AddColumn "endTime"                 "{{ dateRfc3339 .EndTime }}" }}
      {{- $c := $c.AddColumn "endpointSelector"        "{{ if .ReportSpec.Endpoints }}{{ .ReportSpec.Endpoints.Selector }}{{ end }}" }}
      {{- $c := $c.AddColumn "namespaceNames"          "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.Namespaces }}{{ join \";\" .ReportSpec.Endpoints.Namespaces.Names }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "namespaceSelector"       "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.Namespaces }}{{ .ReportSpec.Endpoints.Namespaces.Selector }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "serviceAccountNames"     "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.ServiceAccounts }}{{ join \";\" .ReportSpec.Endpoints.ServiceAccounts.Names }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "serviceAccountSelectors" "{{ if .ReportSpec.Endpoints }}{{ if .ReportSpec.Endpoints.ServiceAccounts }}{{ .ReportSpec.Endpoints.ServiceAccounts.Selector }}{{ end }}{{ end }}" }}
      {{- $c := $c.AddColumn "numCreatedPolicies"      "{{ .AuditSummary.NumCreate }}" }}
      {{- $c := $c.AddColumn "numModifiedPolicies"     "{{ .AuditSummary.NumModify }}" }}
      {{- $c := $c.AddColumn "numDeletedPolicies"      "{{ .AuditSummary.NumDelete }}" }}
      {{- $c.Render . }}
`,
				},
				{
					Name:     "events.json",
					Template: "{{ toJson .AuditEvents }}",
				},
				{
					Name:     "events.yaml",
					Template: "{{ toYaml .AuditEvents }}",
				},
			},
			UISummaryTemplate: v3.ReportTemplate{
				Name: "ui-summary.json",
				Template: `
    {"heading":"Network Policy Configuration Changes","type":"panel","widgets":[{"data":[{"label":"Created","value":{{.AuditSummary.NumCreate }}}],"heading":"Network Policies","summary":{"label":"Total","total":{{.AuditSummary.NumTotal }}},"type":"radialbarchart"},{"data":[{"label":"Modified","value":{{.AuditSummary.NumModify }}}],"heading":"Network Policies","summary":{"label":"Total","total":{{.AuditSummary.NumTotal }}},"type":"radialbarchart"},{"data":[{"label":"Deleted","value":{{.AuditSummary.NumDelete }}}],"heading":"Network Policies","summary":{"label":"Total","total":{{.AuditSummary.NumTotal }}},"type":"radialbarchart"}]}
`,
			},
		},
	}
}

func (c *complianceComponent) complianceGlobalReportCISBenchmark() *v3.GlobalReportType {
	return &v3.GlobalReportType{
		TypeMeta: metav1.TypeMeta{Kind: "GlobalReportType", APIVersion: "projectcalico.org/v3"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cis-benchmark",
			Labels: map[string]string{
				"global-report-type": "cis-benchmark",
			},
		},
		Spec: v3.ReportTypeSpec{
			DownloadTemplates: []v3.ReportTemplate{
				{
					Name: "all-tests.csv",
					Template: `nodeName,testIndex,status,scored
{{ range $i, $node := .CISBenchmark -}}
{{- range $j, $section := $node.Results -}}
{{- range $k, $result := $section.Results -}}
{{- $node.NodeName }},{{ $result.TestNumber }},{{ $result.Status }},{{ $result.Scored }}
{{ end }}
{{- end }}
{{- end }}`,
				},
				{
					Name: "failed-tests.csv",
					Template: `nodeName,testIndex,status,scored
{{ range $i, $node := .CISBenchmark }}
{{- range $j, $section := $node.Results }}
{{- range $k, $result := $section.Results }}
{{- if eq $result.Status "FAIL" }}
{{- $node.NodeName }},{{ $result.TestNumber }},{{ $result.Status }},{{ $result.Scored }}
{{ end }}
{{- end }}
{{- end }}
{{- end }}`,
				},
			},
			IncludeCISBenchmarkData: true,
			UISummaryTemplate: v3.ReportTemplate{
				Name:     "ui-summary.json",
				Template: `{{ $n := len .CISBenchmark }}{"heading": "Kubernetes CIS Benchmark","type":"row","widgets": [{"heading": "Node Failure Summary","type":"cis-benchmark-nodes","summary": {"label": "Total","total":{{ $n }}},"data": [{"label": "HIGH","value":{{ .CISBenchmarkSummary.HighCount }},"desc": "Nodes with {{ if .ReportSpec.CIS }}{{ if .ReportSpec.CIS.HighThreshold }}{{ .ReportSpec.CIS.HighThreshold }}{{ else }}100{{ end }}{{ else }}100{{ end }}% or more tests passing"}, {"label": "MED","value": {{ .CISBenchmarkSummary.MedCount }},"desc": "Nodes with {{ if .ReportSpec.CIS }}{{ if .ReportSpec.CIS.MedThreshold }}{{ .ReportSpec.CIS.MedThreshold }}{{ else }}50{{ end }}{{ else }}50{{ end }}% or more tests passing"}, {"label": "LOW","value": {{ .CISBenchmarkSummary.LowCount }},"desc": "Nodes with less than {{ if .ReportSpec.CIS }}{{ if .ReportSpec.CIS.MedThreshold }}{{ .ReportSpec.CIS.MedThreshold }}{{ else }}50{{ end }}{{ else }}50{{ end }}% tests passing"}]}{{ if .CISBenchmark }}, {"heading": "Top Failed Tests","type": "cis-benchmark-tests","topFailedTests": {"tests": [{{ $tests := cisTopFailedTests . }}{{ $nTests := len $tests }}{{ range $i, $test := $tests }}{"index": "{{ $test.TestNumber }}","description": "{{ $test.TestDesc }}","failedCount": "{{ $test.Count }}"} {{ $i1 := add1 $i }}{{ if ne $i1 $nTests }}, {{ end }}{{ end }}]} }{{ end }}]}`,
			},
		},
	}
}
