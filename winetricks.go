package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var winetricksPattern = regexp.MustCompile(`^[a-zA-Z_0-9-]+(=[a-zA-Z0-9]*)?$`)

// Settings are repeatable. Installed dependency verbs are recorded in the log.
var repeatableSettings = map[string]bool{}

func init() {
	for _, s := range strings.Fields(`alldlls=builtin alldlls=default autostart_winedbg=disabled autostart_winedbg=enabled bad cfc=disabled cfc=enabled csmt=force csmt=off csmt=on fontfix fontsmooth=bgr fontsmooth=disable fontsmooth=gray fontsmooth=rgb forcemono good grabfullscreen=n grabfullscreen=y gsm=0 gsm=1 gsm=2 gsm=3 heapcheck hidewineexports=disable hidewineexports=enable hosts isolate_home macdriver=mac macdriver=x11 mackeyremap=both mackeyremap=left mackeyremap=none mimeassoc=off mimeassoc=on mwo=disable mwo=enabled mwo=force native_mdac native_oleaut32 nocrashdialog npm=repack nt351 nt40 orm=backbuffer orm=fbo psm=0 psm=1 psm=2 psm=3 remove_mono renderer=gdi renderer=gl renderer=no3d renderer=vulkan rtlm=auto rtlm=disabled rtlm=readdraw rtlm=readtex rtlm=texdraw rtlm=textex sandbox set_mididevice set_userpath shader_backend=arb shader_backend=glsl shader_backend=none sound=alsa sound=coreaudio sound=oss sound=disabled sound=pulse ssm=disabled ssm=enabled usetakefocus=n usetakefocus=y vd=1024x768 vd=1280x1024 vd=1440x900 vd=640x480 vd=800x600 vd=off videomemorysize=1024 videomemorysize=2048 videomemorysize=512 videomemorysize=default vista vsm=0 vsm=1 vsm=2 vsm=3 win10 win11 win20 win2k3 win2k8r2 win2k8 win2k win30 win31 win7 win81 win8 win95 win98 windowmanagerdecorated=n windowmanagerdecorated=y windowmanagermanaged=n windowmanagermanaged=y winme winver= winxp`) {
		repeatableSettings[s] = true
	}
}
func checkWinetricks(args []string, prefix string) error {
	if len(args) == 0 {
		return fmt.Errorf("no winetricks verb specified")
	}
	requested := map[string]bool{}
	for _, arg := range args {
		if !winetricksPattern.MatchString(arg) && !(strings.Contains(arg, ".") && regular(arg)) {
			return fmt.Errorf("invalid winetricks verb %q", arg)
		}
		requested[arg] = true
	}
	if prefix == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(prefix, "winetricks.log"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		verb := strings.TrimSpace(line)
		if requested[verb] && !repeatableSettings[verb] {
			return fmt.Errorf("winetricks verb %q is already installed", verb)
		}
	}
	return nil
}
