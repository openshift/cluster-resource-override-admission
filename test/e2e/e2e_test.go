package e2e

import (
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestClusterResourceOverrideAdmission(t *testing.T) {
	f := &fixture{client: options.client}

	f.MustHaveAdmissionRegistrationV1beta1(t)
	f.MustHaveClusterResourceOverrideAdmissionConfiguration(t)

	// The test assumes the following configuration
	// memoryRequestToLimitPercent: 50
	// cpuRequestToLimitPercent: 25
	// limitCPUToMemoryPercent: 200

	tests := []struct {
		name string
		request *corev1.Pod
		assert func(t *testing.T, got *corev1.Pod)
	}{
		{
			name: "WithMultipleContainers",
			request: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "pod-",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						corev1.Container{
							Name: "db",
							Image: "docker.io/tohinkashem/echo-operator:latest",
							Ports: []corev1.ContainerPort {
								corev1.ContainerPort{
									Name: "db",
									ContainerPort: 60000,
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1024Mi"),
									corev1.ResourceCPU:    resource.MustParse("1000m"),
								},
							},
						},
						corev1.Container{
							Name: "app",
							Image: "docker.io/tohinkashem/echo-operator:latest",
							Ports: []corev1.ContainerPort {
								corev1.ContainerPort{
									Name: "app",
									ContainerPort: 60100,
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, got *corev1.Pod) {
				// verify db container
				dbRequests := got.Spec.Containers[0].Resources.Requests
				dbLimits := got.Spec.Containers[0].Resources.Limits
				validate(t, dbLimits, corev1.ResourceCPU, resource.MustParse("2000m"))
				validate(t, dbRequests, corev1.ResourceMemory, resource.MustParse("512Mi"))
				validate(t, dbRequests, corev1.ResourceCPU, resource.MustParse("500m"))

				// verify app container
				appRequests := got.Spec.Containers[1].Resources.Requests
				appLimits := got.Spec.Containers[1].Resources.Limits
				validate(t, appLimits, corev1.ResourceCPU, resource.MustParse("1000m"))
				validate(t, appRequests, corev1.ResourceMemory, resource.MustParse("256Mi"))
				validate(t, appRequests, corev1.ResourceCPU, resource.MustParse("250m"))
			},
		},
		{
			name: "WithInitContainer",
			request: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "pod-",
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						corev1.Container{
							Name: "init",
							Image: "busybox:latest",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1024Mi"),
									corev1.ResourceCPU:    resource.MustParse("1000m"),
								},
							},
							Command: []string{
								"sh",
								"-c",
								"echo The app is running! && sleep 1",
							},
						},
					},
					Containers: []corev1.Container{
						corev1.Container{
							Name: "app",
							Image: "docker.io/tohinkashem/echo-operator:latest",
							Ports: []corev1.ContainerPort {
								corev1.ContainerPort{
									Name: "app",
									ContainerPort: 60100,
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),								},
							},
						},
					},
				},
			},
			assert: func(t *testing.T, got *corev1.Pod) {
				// verify init container
				initRequests := got.Spec.InitContainers[0].Resources.Requests
				initLimits := got.Spec.InitContainers[0].Resources.Limits
				validate(t, initLimits, corev1.ResourceCPU, resource.MustParse("2000m"))
				validate(t, initRequests, corev1.ResourceMemory, resource.MustParse("512Mi"))
				validate(t, initRequests, corev1.ResourceCPU, resource.MustParse("500m"))

				// verify app container
				appRequests := got.Spec.Containers[0].Resources.Requests
				appLimits := got.Spec.Containers[0].Resources.Limits
				validate(t, appLimits, corev1.ResourceCPU, resource.MustParse("1000m"))
				validate(t, appRequests, corev1.ResourceMemory, resource.MustParse("256Mi"))
				validate(t, appRequests, corev1.ResourceCPU, resource.MustParse("250m"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := options.client
			podGot, errGot := client.CoreV1().Pods(options.namespace).Create(test.request)
			require.NoError(t, errGot)

			defer func() {
				p, err := client.CoreV1().Pods(options.namespace).Get(podGot.Name, metav1.GetOptions{})
				require.NoError(t, err, "cleaning up - pod not found")

				err = client.CoreV1().Pods(options.namespace).Delete(p.Name, &metav1.DeleteOptions{})
				require.NoErrorf(t, err, "cleaning up - failed to delete pod - %v", err)
			}()

			test.assert(t, podGot)
		})
	}
}

func validate(t *testing.T, list corev1.ResourceList, name corev1.ResourceName, want resource.Quantity) {
	got, ok := list[corev1.ResourceName(name)]
	require.Truef(t, ok, "expected: %s, now absent", name)

	result := got.Equal(want)
	require.True(t, result, "mutated, expected: %v, got %v", want, got)
}
