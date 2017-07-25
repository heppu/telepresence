package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Pod struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Metadata   struct {
	} `json:"metadata"`
	Items []struct {
		Kind       string `json:"kind"`
		APIVersion string `json:"apiVersion"`
		Metadata   struct {
			Name              string    `json:"name"`
			GenerateName      string    `json:"generateName"`
			Namespace         string    `json:"namespace"`
			SelfLink          string    `json:"selfLink"`
			UID               string    `json:"uid"`
			ResourceVersion   string    `json:"resourceVersion"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
			Labels            struct {
				Name            string `json:"name"`
				PodTemplateHash string `json:"pod-template-hash"`
			} `json:"labels"`
			Annotations struct {
				KubernetesIoCreatedBy string `json:"kubernetes.io/created-by"`
			} `json:"annotations"`
			OwnerReferences []struct {
				APIVersion string `json:"apiVersion"`
				Kind       string `json:"kind"`
				Name       string `json:"name"`
				UID        string `json:"uid"`
				Controller bool   `json:"controller"`
			} `json:"ownerReferences"`
		} `json:"metadata"`
		Spec struct {
			Volumes []struct {
				Name   string `json:"name"`
				Secret struct {
					SecretName  string `json:"secretName"`
					DefaultMode int    `json:"defaultMode"`
				} `json:"secret"`
			} `json:"volumes"`
			Containers []struct {
				Name      string `json:"name"`
				Image     string `json:"image"`
				Resources struct {
				} `json:"resources"`
				VolumeMounts []struct {
					Name      string `json:"name"`
					ReadOnly  bool   `json:"readOnly"`
					MountPath string `json:"mountPath"`
				} `json:"volumeMounts"`
				TerminationMessagePath string `json:"terminationMessagePath"`
				ImagePullPolicy        string `json:"imagePullPolicy"`
			} `json:"containers"`
			RestartPolicy                 string `json:"restartPolicy"`
			TerminationGracePeriodSeconds int    `json:"terminationGracePeriodSeconds"`
			DNSPolicy                     string `json:"dnsPolicy"`
			ServiceAccountName            string `json:"serviceAccountName"`
			ServiceAccount                string `json:"serviceAccount"`
			NodeName                      string `json:"nodeName"`
			SecurityContext               struct {
			} `json:"securityContext"`
		} `json:"spec"`
		Status struct {
			Phase      string `json:"phase"`
			Conditions []struct {
				Type               string      `json:"type"`
				Status             string      `json:"status"`
				LastProbeTime      interface{} `json:"lastProbeTime"`
				LastTransitionTime time.Time   `json:"lastTransitionTime"`
			} `json:"conditions"`
			HostIP            string    `json:"hostIP"`
			PodIP             string    `json:"podIP"`
			StartTime         time.Time `json:"startTime"`
			ContainerStatuses []struct {
				Name  string `json:"name"`
				State struct {
					Running struct {
						StartedAt time.Time `json:"startedAt"`
					} `json:"running"`
				} `json:"state"`
				LastState struct {
				} `json:"lastState"`
				Ready        bool   `json:"ready"`
				RestartCount int    `json:"restartCount"`
				Image        string `json:"image"`
				ImageID      string `json:"imageID"`
				ContainerID  string `json:"containerID"`
			} `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

type DockerNetwork struct {
	Subnet  string `json:"Subnet"`
	Gateway string `json:"Gateway"`
}

const (
	SSH_PORT   = "9876"
	TELE_IMAGE = "datawire/telepresence-local:0.58"
)

func main() {
	out, err := exec.Command("kubectl", "config", "current-context").Output()
	if err != nil {
		fmt.Printf("Could not get current context for kubectl: %s\n", err)
		return
	}
	context := strings.TrimSpace(string(out))
	fmt.Println("Using context:", context)

	if _, err = exec.Command("kubectl", "cluster-info").Output(); err != nil {
		fmt.Printf("Could not connect to cluster: %s\n", err)
		return
	}

	fmt.Println("Deploying...")
	if _, err = exec.Command("kubectl", "apply", "-f", "tele.yaml").Output(); err != nil {
		fmt.Printf("Could not deploy tele image: %s\n", err)
		return
	}

	defer func() {
		fmt.Println("Deleting deployment")
		if _, err = exec.Command("kubectl", "delete", "-f", "tele.yaml").Output(); err != nil {
			fmt.Printf("Deleting deployment failed: %s\n", err)
			return
		}
		fmt.Println("Exited succesfully")
	}()

	pod := &Pod{}

Wait:
	for {
		if out, err = exec.Command("kubectl", "get", "pod", "-l", "name=myservice", "-o", "json").Output(); err != nil {
			fmt.Printf("Could not get pod info: %s\n", err)
			return
		}

		if err = json.Unmarshal(out, pod); err != nil {
			fmt.Println(err)
			return
		}

		if len(pod.Items) != 1 {
			fmt.Printf("Invalid amount of pods %d, expecting 1\n", len(pod.Items))
			return
		}

		switch pod.Items[0].Status.Phase {
		case "Running":
			fmt.Printf("Pod %s is up and running\n", pod.Items[0].Metadata.Name)
			break Wait
		case "Pending":
			continue
		default:
			fmt.Printf("Pod in unexpected state %q\n", pod.Items[0].Status.Phase)
			return
		}
	}

	pfCmd := exec.Command("kubectl", "port-forward'", pod.Items[0].Metadata.Name, SSH_PORT+":8022")
	if err = pfCmd.Start(); err != nil {
		fmt.Printf("Could not port forward: %s\n", err)
		return
	}
	defer func() {
		if err := pfCmd.Process.Kill(); err != nil {
			fmt.Println("Could not close port forward:", err)
			return
		}
		fmt.Println("Port forward closed")
	}()

	if out, err = exec.Command("docker", "network", "inspect", "bridge", "--format={{json .IPAM.Config}}").Output(); err != nil {
		fmt.Printf("Could not get ip for docker0: %s\n", err)
		return
	}

	networks := []DockerNetwork{}
	if err = json.Unmarshal(out, &networks); err != nil {
		fmt.Println(err)
		return
	}

	if len(networks) == 0 {
		fmt.Println("Could not find ip for docker0")
		return
	}
	dockerIP := networks[0].Gateway
	fmt.Println("IP for docker0:", dockerIP)

	socatCmd := exec.Command(
		"socat",
		fmt.Sprintf("TCP4-LISTEN:%s,bind=%s,reuseaddr,fork", SSH_PORT, dockerIP),
		fmt.Sprintf("TCP4:127.0.0.1:%s", SSH_PORT),
	)

	if err = socatCmd.Start(); err != nil {
		fmt.Printf("Could not start socat: %s\n", err)
		return
	}
	defer func() {
		if err := pfCmd.Process.Kill(); err != nil {
			fmt.Println("Could not close socat:", err)
			return
		}
		fmt.Println("socat closed closed")
	}()

	if out, err = exec.Command("kubectl", "exec", pod.Items[0].Metadata.Name, "env").Output(); err != nil {
		fmt.Printf("Could not get env variables: %s\n", err)
		return
	}
	env := out
}
