package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubemindsv1alpha1 "kubeminds/api/v1alpha1"
)

var _ = Describe("DiagnosisTask Controller", func() {
	Context("When reconciling a DiagnosisTask", func() {
		It("should update Status from Pending to Running", func() {
			By("Creating a new DiagnosisTask")
			task := &kubemindsv1alpha1.DiagnosisTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "default",
				},
				Spec: kubemindsv1alpha1.DiagnosisTaskSpec{
					Target: kubemindsv1alpha1.DiagnosisTarget{
						Namespace: "default",
						Name:      "test-pod",
						Kind:      "Pod",
					},
					Policy: kubemindsv1alpha1.DiagnosisPolicy{
						MaxSteps: 5,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), task)).To(Succeed())

			// Wait for Status to become Running
			// This test will fail if CRDs are not installed in the EnvTest cluster
			// And if we didn't inject a mock LLM provider, it might hang or fail on API call
			// But for now we just check the transition from Pending -> Running which happens *before* LLM call
			Eventually(func() string {
				var t kubemindsv1alpha1.DiagnosisTask
				err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(task), &t)
				if err != nil {
					return ""
				}
				return string(t.Status.Phase)
			}, time.Second*10, time.Millisecond*500).Should(Or(Equal(string(kubemindsv1alpha1.PhaseRunning)), Equal(string(kubemindsv1alpha1.PhaseCompleted))))
		})
	})
})
