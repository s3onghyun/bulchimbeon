//go:build windows

// Package windows is the placeholder for the Windows backend. Nothing is implemented yet:
// every source reports platform.ErrNotImplemented, so `bulchimbeon run` refuses to start.
//
// Plan, for whoever picks this up:
//   - sleep lock: SetThreadExecutionState(ES_CONTINUOUS | ES_SYSTEM_REQUIRED) from one
//     locked OS thread (runtime.LockOSThread), cleared with ES_CONTINUOUS on release. The
//     state dies with the thread, so a crash releases it like the Linux pipe does.
//     `powercfg /requests` lists it for the "held" check and for other holders.
//   - processes: CreateToolhelp32Snapshot for PIDs and parents, QueryFullProcessImageName
//     for the executable, GetProcessTimes for CPU time; arguments for runners only.
//   - battery: GetSystemPowerStatus. Heat: no stable user-mode API, so report normal.
//
// The engine and its fixtures (testdata/engine) are shared, so only this package changes.
package windows

import (
	"io"

	"github.com/s3onghyun/bulchimbeon/internal/engine"
	"github.com/s3onghyun/bulchimbeon/internal/platform"
)

// New returns the Windows backend stub.
func New() platform.Platform {
	return platform.Platform{
		Name:      "windows",
		Processes: processes{},
		Inhibitor: &inhibitor{},
		Battery:   battery{},
		Thermal:   thermal{},
		Holders:   holders{},
		Doctor: func(w io.Writer, lang engine.Lang) bool {
			io.WriteString(w, engine.T(lang, "Windows 는 아직 지원하지 않습니다.\n", "Windows is not supported yet.\n", "暂不支持 Windows。\n"))
			return false
		},
	}
}

type processes struct{}

func (processes) Scan([]engine.AgentKind) (platform.ProcessScan, error) {
	return platform.ProcessScan{}, platform.ErrNotImplemented
}

// inhibitor will wrap SetThreadExecutionState.
type inhibitor struct{ wanted bool }

func (i *inhibitor) Acquire()      { i.wanted = true }
func (i *inhibitor) Release()      { i.wanted = false }
func (i *inhibitor) Refresh() bool { return false }
func (i *inhibitor) Status() platform.LockStatus {
	st := platform.LockStatus{Wanted: i.wanted, Method: "SetThreadExecutionState (not implemented)"}
	if i.wanted {
		st.Blocks = []platform.Block{{What: "system", Error: platform.ErrNotImplemented.Error()}}
	}
	return st
}

type battery struct{}

func (battery) Battery() (*engine.BatteryStatus, error) { return nil, platform.ErrNotImplemented }

type thermal struct{}

func (thermal) Thermal() platform.ThermalReading {
	return platform.ThermalReading{Level: engine.HeatNormal, LevelName: engine.HeatNormal.String()}
}

type holders struct{}

func (holders) Holders() ([]platform.Holder, error) { return nil, platform.ErrNotImplemented }
