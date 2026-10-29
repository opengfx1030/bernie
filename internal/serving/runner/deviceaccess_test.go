package runner

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
)

// par1GPUs are the GPUs of par1-cs25: the HIP order of the KFD nodes and
// the PCI address of every card. The two orders differ, so the CDI index
// (the PCI order) is not the HIP index.
func par1GPUs() []gpu.GPU {
	pci := []string{
		"0000:07:00.0", "0000:c7:00.0", "0000:83:00.0", "0000:4a:00.0", "0000:47:00.0",
		"0000:0a:00.0", "0000:12:00.0", "0000:15:00.0", "0000:52:00.0", "0000:55:00.0",
	}
	out := make([]gpu.GPU, len(pci))
	for i, p := range pci {
		out[i] = gpu.GPU{HIPIndex: i, PCI: p}
	}
	return out
}

// The CDI index of a GPU is its rank in the PCI-address order of the host.
func TestCDIDeviceNames(t *testing.T) {
	all := par1GPUs()
	tests := []struct {
		name     string
		selected []int
		want     []string
	}{
		{"one GPU", []int{4}, []string{"amd.com/gpu=4"}},
		{"the first GPU", []int{0}, []string{"amd.com/gpu=0"}},
		{"two GPUs", []int{0, 1}, []string{"amd.com/gpu=0", "amd.com/gpu=9"}},
		{"every GPU", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, []string{"amd.com/gpu=all"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cdiDeviceNames(all, tt.selected)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// A GPU without a PCI address cannot be mapped: bernie does not guess.
func TestCDIDeviceNamesWithoutPCI(t *testing.T) {
	all := par1GPUs()
	all[2].PCI = ""
	if _, err := cdiDeviceNames(all, []int{2}); err == nil {
		t.Error("a GPU without a PCI address must not be mapped")
	}
}

// The classic form passes the device nodes of the host, the CDI form the
// devices of the spec. A run never mixes them.
func TestContainerCommandDeviceAccess(t *testing.T) {
	classic := plan("cli")
	got := ContainerCommand(classic, host.Engine{Name: "docker"}, "", "")
	if !containsPair(got, "--device", "/dev/kfd") || !containsPair(got, "--device", "/dev/dri") {
		t.Errorf("the classic form must pass the device nodes: %q", got)
	}

	cdi := plan("cli")
	cdi.DeviceAccess = host.DeviceAccessCDI
	cdi.CDIDevices = []string{"amd.com/gpu=0", "amd.com/gpu=9"}
	got = ContainerCommand(cdi, host.Engine{Name: "docker"}, "", "")
	if !containsPair(got, "--device", "amd.com/gpu=0") || !containsPair(got, "--device", "amd.com/gpu=9") {
		t.Errorf("the CDI form must pass the CDI devices: %q", got)
	}
	if containsPair(got, "--device", "/dev/dri") {
		t.Errorf("the CDI form must not pass the device nodes: %q", got)
	}
	// The group flags stay: the CDI spec does not add the groups of the
	// user to the container process.
	if !containsPair(got, "--group-add", "39") {
		t.Errorf("the group flags must stay in CDI mode: %q", got)
	}
}

// The Compose file carries the same devices as the command.
func TestComposeDeviceAccess(t *testing.T) {
	cdi := plan("cli")
	cdi.DeviceAccess = host.DeviceAccessCDI
	cdi.CDIDevices = []string{"amd.com/gpu=0"}
	b, err := Compose(cdi, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			Devices []string `yaml:"devices"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.Services["vllm"].Devices; !reflect.DeepEqual(got, []string{"amd.com/gpu=0"}) {
		t.Errorf("devices = %q", got)
	}
	if !strings.Contains(string(b), "not confirmed on a live host yet") {
		t.Errorf("the compose file must say that the CDI names are unverified:\n%s", b)
	}
}

// containsPair reports whether the argv holds the flag with the value.
func containsPair(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

// The plan of a container run carries the mode, the reason and the CDI
// devices.
func TestPlanDeviceAccessFields(t *testing.T) {
	p := &recipe.Plan{DeviceAccess: host.DeviceAccessCDI, DeviceAccessReason: "the spec fits this host",
		CDIDevices: []string{"amd.com/gpu=0"}}
	if p.DeviceAccess != "cdi" || p.DeviceAccessReason == "" || len(p.CDIDevices) != 1 {
		t.Errorf("plan fields: %+v", p)
	}
}
