package main

import (
	"context"
	"flag"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
	"time"
)

func main() {

	customPath := os.Getenv("KUBECONFIG")
	var kubeconfig *string
	if customPath != "" {
		kubeconfig = flag.String("kubeconfig", customPath, "(optional) absolute path to the kubeconfig file")
	} else if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))

	var podsPerNode = make(map[string]int)
	var podsOnNode = make(map[string][]v1.Pod)
	for _, pod := range pods.Items {
		fmt.Printf("Pod name: %s, node: %s\n", pod.GetName(), pod.Spec.NodeName)
		podsPerNode[pod.Spec.NodeName]++
		podsOnNode[pod.Spec.NodeName] = append(podsOnNode[pod.Spec.NodeName], pod)
	}

	idealPodsPerNode := len(pods.Items) / len(podsPerNode)
	fmt.Printf("Ideal pods per node: %d\n", idealPodsPerNode)

	if idealPodsPerNode > 5 {
		idealPodsPerNode -= 5
	}

	fmt.Printf("Ideal pods per node (adjusted): %d\n", idealPodsPerNode)

	adjustmentsPerNode := make(map[string]int)
	for node, count := range podsPerNode {
		adjustmentsPerNode[node] = count - idealPodsPerNode
	}

	fmt.Println("Pods per node:")
	for node, count := range podsPerNode {
		fmt.Printf("%s: %d. Adjustment: %d\n", node, count, adjustmentsPerNode[node])
	}

	for node, count := range podsPerNode {
		adjustment := adjustmentsPerNode[node]
		if adjustment > 0 {
			fmt.Printf("Node %s has %d pods, more than ideal [%d]\n", node, count, idealPodsPerNode)
			selectedPods := selectPods(podsOnNode[node], adjustmentsPerNode[node])
			fmt.Printf("Selected %d pods to delete\n", len(selectedPods))
			for _, pod := range selectedPods {
				fmt.Printf("Deleting pod %s\n", pod.GetName())
				err := clientset.CoreV1().Pods(pod.GetNamespace()).Delete(context.TODO(), pod.GetName(), metav1.DeleteOptions{})
				time.Sleep(1 * time.Second)
				if err != nil {
					fmt.Printf("Error deleting pod %s: %s\n", pod.GetName(), err.Error())
				}
			}
		}
	}
}

func selectPods(pods []v1.Pod, numberOfPodsToSelect int) []v1.Pod {
	var selectedPods []v1.Pod
	for _, pod := range pods {
		if len(selectedPods) >= numberOfPodsToSelect {
			break
		}

		if isPodPartOfDeployment(pod) {
			fmt.Printf("Pod %s is part of a deployment\n", pod.GetName())
			selectedPods = append(selectedPods, pod)
		}
	}

	return selectedPods
}

func isPodPartOfDeployment(pod v1.Pod) bool {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == "ReplicaSet" {
			return true
		}
	}
	return false
}
