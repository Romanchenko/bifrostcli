package cmd

import (
	"context"
	"flag"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	v1 "k8s.io/client-go/applyconfigurations/apps/v1"
	v13 "k8s.io/client-go/applyconfigurations/core/v1"
	v12 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"log"
	"path/filepath"
)

var ImageName string
var SecretName string

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Deploy bifrost pod and daemon-set of telemetry-brokers",
	Long: `Deploys default environment for export of metrics to the clickhouse instances. It will create a daemon set on 
your current cluster and a central controller which regulates parameters of export. You need a mongo storage for 
the support of metrics.
`,
	Run: func(cmd *cobra.Command, args []string) {
		var kubeconfig *string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}
		flag.Parse()

		// use the current context in kubeconfig
		config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err.Error())
		}

		// create the clientset
		clientset, err := kubernetes.NewForConfig(config)
		telemetryNamespace := "telemetry"
		telemetryBroker := "telemetry-broker"
		if err != nil {
			panic(err.Error())
		}

		log.Println("Creating daemonSet")

		imageName := ImageName
		log.Println("Using image: ", imageName)
		secretName := SecretName
		log.Println("Will fetch secrets from: ", secretName)

		retrievalType := v13.EnvVarApplyConfiguration{}
		(&retrievalType).WithName("RETRIEVAL_TYPE").WithValue("standalone")

		podName := v13.EnvVarApplyConfiguration{}
		(&podName).
			WithName("CURRENT_NODE").
			WithValueFrom(&v13.EnvVarSourceApplyConfiguration{
				FieldRef: (&v13.ObjectFieldSelectorApplyConfiguration{}).WithFieldPath("spec.nodeName"),
			})

		containerPort := v13.ContainerPortApplyConfiguration{}
		(&containerPort).WithContainerPort(8080)
		pullPolicy := corev1.PullAlways
		podSpec := (&v13.PodSpecApplyConfiguration{}).
			WithContainers(&v13.ContainerApplyConfiguration{
				Image:           &imageName,
				ImagePullPolicy: &pullPolicy,
				Name:            &telemetryBroker,
				EnvFrom:         []v13.EnvFromSourceApplyConfiguration{{SecretRef: (&v13.SecretEnvSourceApplyConfiguration{}).WithName(secretName)}},
				Env: []v13.EnvVarApplyConfiguration{
					retrievalType, podName,
				},
				Ports: []v13.ContainerPortApplyConfiguration{containerPort},
			})
		daemonSet := v1.DaemonSet(telemetryBroker, telemetryNamespace)
		daemonSet = daemonSet.WithLabels(map[string]string{"app": telemetryBroker})
		daemonSet = daemonSet.WithSpec(&v1.DaemonSetSpecApplyConfiguration{
			Selector: &v12.LabelSelectorApplyConfiguration{MatchLabels: map[string]string{"app": telemetryBroker}},
			Template: (&v13.PodTemplateSpecApplyConfiguration{}).
				WithNamespace(telemetryNamespace).
				WithLabels(map[string]string{"app": telemetryBroker, "version": "v1"}).
				WithSpec(podSpec),
		})

		_, err = clientset.AppsV1().DaemonSets(telemetryNamespace).
			Apply(context.TODO(), daemonSet, metav1.ApplyOptions{FieldManager: "system"})
		if err != nil {
			log.Fatalln("Failed to create daemonSet for telemetry broker, error: ", err)
			return
		}
		log.Println("Successfully created daemonSet")

		log.Println("Creating service")

		service := v13.Service(telemetryBroker, telemetryNamespace)
		servicePort := v13.ServicePortApplyConfiguration{}
		(&servicePort).WithName("http").WithPort(8080).WithTargetPort(intstr.IntOrString{IntVal: 8080})

		service.
			WithLabels(map[string]string{"app": telemetryBroker}).
			WithSpec(&v13.ServiceSpecApplyConfiguration{
				Ports:    []v13.ServicePortApplyConfiguration{servicePort},
				Selector: map[string]string{"app": telemetryBroker},
			})
		_, err = clientset.CoreV1().Services(telemetryNamespace).Apply(context.TODO(), service, metav1.ApplyOptions{FieldManager: "system"})
		if err != nil {
			log.Fatalln("Failed to create service: ", err)
			return
		}
		log.Println("Successfully created service ", telemetryBroker)
	},
}

func init() {
	rootCmd.AddCommand(installCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// installCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// installCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	installCmd.Flags().StringVarP(
		&ImageName,
		"imageName",
		"i",
		"apollin/telemetry-broker-sandbox:v4",
		"Select version of telemetry broker you wish to use",
	)

	installCmd.Flags().StringVarP(
		&SecretName,
		"secretName",
		"s",
		"mongo-connection-string",
		"If you want to rename your k8s secret, change this parameter to the one you've chosen")
}
