package storage

import "testing"

func TestParseMacMountPathContainingType(t *testing.T) {
	got, err := ParseMountOutput("//server/team on /Volumes/research type data (smbfs, nodev, mounted by demo)", "/Volumes/research type data/project")
	if err != nil {
		t.Fatal(err)
	}
	if got.MountPoint != "/Volumes/research type data" || got.Type != "smbfs" || !got.IsSMB() {
		t.Fatalf("unexpected mount: %+v", got)
	}
}

func TestParseLinuxMountOutput(t *testing.T) {
	output := "/dev/root on / type ext4 (rw,relatime)\n//server/team on /mnt/team type cifs (rw,relatime)\n"
	for _, tc := range []struct {
		root, point, kind string
		smb               bool
	}{
		{"/tmp/doctor/storage", "/", "ext4", false},
		{"/mnt/team/project", "/mnt/team", "cifs", true},
		{"/mnt/team-other", "/", "ext4", false},
	} {
		t.Run(tc.root, func(t *testing.T) {
			got, err := ParseMountOutput(output, tc.root)
			if err != nil {
				t.Fatal(err)
			}
			if got.MountPoint != tc.point || got.Type != tc.kind || got.IsSMB() != tc.smb {
				t.Fatalf("got %+v (SMB=%v), want %s %s SMB=%v", got, got.IsSMB(), tc.point, tc.kind, tc.smb)
			}
		})
	}
}
