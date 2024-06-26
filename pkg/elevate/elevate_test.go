package elevate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"k8s.io/client-go/tools/clientcmd/api"
)

func fakeExecCommandError(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcessError", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...) //#nosec: G204
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func fakeExecCommandSuccess(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcessSuccess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...) //#nosec: G204
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

var fakeAPIConfig = api.Config {
	Kind:        "Config",
	APIVersion:  "v1",
	Preferences: api.Preferences{},
	Clusters: map[string]*api.Cluster{
		"dummy_cluster": {
			Server: "https://api-backplane.apps.something.com/backplane/cluster/configcluster",
		},
	},
	AuthInfos: map[string]*api.AuthInfo{
		"anonymous": {
			LocationOfOrigin: "England",
		},
	},
	Contexts: map[string]*api.Context{
		"default/test123/anonymous": {
			Cluster:   "dummy_cluster",
			Namespace: "default",
			AuthInfo:  "anonymous",
		},
	},
	CurrentContext: "default/test123/anonymous",
}

func fakeReadKubeConfigRaw() (api.Config, error) {
	return fakeAPIConfig, nil
}

func TestHelperProcessError(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(1)
}

func TestHelperProcessSuccess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprintf(os.Stdout, "")
	os.Exit(0)
}

func TestAddElevationReasonToRawKubeconfig(t *testing.T) {
	fakeAPIConfigNoUser := *fakeAPIConfig.DeepCopy()
	delete(fakeAPIConfigNoUser.AuthInfos, "anonymous")
	fakeAPIConfigNoUser.Contexts["default/test123/anonymous"].AuthInfo = ""

	t.Run("It returns an error if there is no current kubeconfig context", func(t *testing.T) {
		if err := AddElevationReasonToRawKubeconfig(api.Config{}, "Production cluster"); err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("it returns an error if there is no user info in kubeconfig", func(t *testing.T) {
		if err := AddElevationReasonToRawKubeconfig(fakeAPIConfigNoUser, "Production cluster"); err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("it succeeds if the auth info exists for the current context", func(t *testing.T) {
		if err := AddElevationReasonToRawKubeconfig(fakeAPIConfig, "Production cluster"); err != nil {
			t.Errorf("Expected no errors, got %v", err)
		}
	})
}

func TestRunElevate(t *testing.T) {
	t.Run("It returns an error if we cannot load the kubeconfig", func(t *testing.T) {
		ExecCmd = exec.Command
		OsRemove = os.Remove
		ReadKubeConfigRaw = func() (api.Config, error) {
			return *api.NewConfig(), errors.New("cannot load kfg")
		}
		if err := RunElevate([]string{}); err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("It returns an error if kubeconfig has no current context", func(t *testing.T) {
		ExecCmd = exec.Command
		OsRemove = os.Remove
		ReadKubeConfigRaw = func() (api.Config, error) {
			return *api.NewConfig(), nil
		}
		if err := RunElevate([]string{"oc", "get pods"}); err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("It returns an error if the exec command has errors", func(t *testing.T) {
		ExecCmd = fakeExecCommandError
		OsRemove = os.Remove
		ReadKubeConfigRaw = fakeReadKubeConfigRaw
		if err := RunElevate([]string{"oc", "get pods"}); err == nil {
			t.Error("Expected error, got nil")
		}
	})

	t.Run("It suceeds if the command succeeds, we can clean up the tmp kubeconfig and KUBECONFIG is still set to previous definbed value", func(t *testing.T) {
		ExecCmd = fakeExecCommandSuccess
		OsRemove = func(name string) error { return nil }
		ReadKubeConfigRaw = fakeReadKubeConfigRaw
		mockKubeconfig := "/tmp/dummy_kubeconfig"
		os.Setenv("KUBECONFIG", mockKubeconfig)
		if err := RunElevate([]string{"oc", "get pods"}); err != nil {
			t.Errorf("Expected no errors, got %v", err)
		}
		if kubeconfigPath, kubeconfigDefined := os.LookupEnv("KUBECONFIG"); ! kubeconfigDefined || kubeconfigPath != mockKubeconfig {
			t.Errorf("Expected KUBECONFIG to be definied to previous value, got %v", kubeconfigPath)
		}
	})

	t.Run("It suceeds if the command succeeds, we can clean up the tmp kubeconfig and KUBECONFIG is still not set", func(t *testing.T) {
		ExecCmd = fakeExecCommandSuccess
		OsRemove = func(name string) error { return nil }
		ReadKubeConfigRaw = fakeReadKubeConfigRaw
		os.Unsetenv("KUBECONFIG")
		if err := RunElevate([]string{"oc", "get pods"}); err != nil {
			t.Errorf("Expected no errors, got %v", err)
		}
		if kubeconfigPath, kubeconfigDefined := os.LookupEnv("KUBECONFIG"); kubeconfigDefined {
			t.Errorf("Expected KUBECONFIG to not be definied as previously, got %v", kubeconfigPath)
		}
	})

}
