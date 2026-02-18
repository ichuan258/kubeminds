package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	kubemindsv1alpha1 "kubeminds/api/v1alpha1"
)

func TestAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Server Suite")
}

var _ = Describe("API Server", func() {
	var (
		server    *Server
		scheme    *runtime.Scheme
		k8sClient client.Client
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		err := kubemindsv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		k8sClient = fakeclient.NewClientBuilder().WithScheme(scheme).Build()
		k8sClientset := fake.NewSimpleClientset()
		server = NewServer(k8sClient, k8sClientset, nil, 8081, logr.Discard())
	})

	Context("Diagnosis Tasks", func() {
		It("should create a task", func() {
			task := kubemindsv1alpha1.DiagnosisTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-task",
					Namespace: "default",
				},
				Spec: kubemindsv1alpha1.DiagnosisTaskSpec{
					Target: kubemindsv1alpha1.DiagnosisTarget{
						Kind: "Pod",
						Name: "nginx",
					},
					AlertContext: &kubemindsv1alpha1.AlertContext{
						Labels: map[string]string{"reason": "OOMKilled"},
					},
				},
			}

			body, _ := json.Marshal(task)
			req, _ := http.NewRequest("POST", "/api/v1/tasks", bytes.NewBuffer(body))
			rr := httptest.NewRecorder()

			// Create a handler for the specific route to test
			handler := http.HandlerFunc(server.createTask)
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusCreated))

			var createdTask kubemindsv1alpha1.DiagnosisTask
			err := json.Unmarshal(rr.Body.Bytes(), &createdTask)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdTask.Name).To(Equal("test-task"))
			// Use string literal since PhasePending constant might not be exported or available in this context
			Expect(string(createdTask.Status.Phase)).To(Equal("Pending"))
		})

		It("should list tasks", func() {
			// Pre-create a task
			task := &kubemindsv1alpha1.DiagnosisTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-task",
					Namespace: "default",
				},
			}
			err := k8sClient.Create(context.Background(), task)
			Expect(err).NotTo(HaveOccurred())

			req, _ := http.NewRequest("GET", "/api/v1/tasks", nil)
			rr := httptest.NewRecorder()

			handler := http.HandlerFunc(server.listTasks)
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())

			items := response["items"].([]interface{})
			Expect(len(items)).To(Equal(1))
		})
	})
})
