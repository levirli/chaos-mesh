package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func main() {
	repoRoot := "/Users/mt/Documents/leviworkspace/chaos-mesh"
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			repoRoot + "/config/crd/bases",
		},
	}

	cfg, err := testEnv.Start()
	if err != nil {
		panic(err)
	}

	kubeconfig := clientcmdapi.NewConfig()
	kubeconfig.Clusters["envtest"] = &clientcmdapi.Cluster{
		Server:                   cfg.Host,
		CertificateAuthorityData: cfg.CAData,
	}
	kubeconfig.AuthInfos["envtest"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: cfg.CertData,
		ClientKeyData:         cfg.KeyData,
	}
	kubeconfig.Contexts["envtest"] = &clientcmdapi.Context{
		Cluster:  "envtest",
		AuthInfo: "envtest",
	}
	kubeconfig.CurrentContext = "envtest"

	path := "/tmp/envtest.kubeconfig"
	if err := clientcmd.WriteToFile(*kubeconfig, path); err != nil {
		testEnv.Stop()
		panic(err)
	}

	fmt.Println("envtest control plane started")
	fmt.Println("kubeconfig written to", path)
	fmt.Println("Press Ctrl+C to stop")

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	fmt.Println("\nstopping envtest control plane")
	if err := testEnv.Stop(); err != nil {
		fmt.Println("error stopping envtest:", err)
	}
}
