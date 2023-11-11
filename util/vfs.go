package util

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
)

func GetProfileBuffer(username string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	var extra_user = ""
	var extra_group = ""

	if username != "root" {
		extra_user = fmt.Sprintf("\n%s:x:1000:1000:%s:/:/bin/ash\n", username, username)
		extra_group = fmt.Sprintf("\n%s:x:1000:\n", username)
	}

	var files = []struct {
		Name, Body string
	}{
		{
			"group",
			fmt.Sprintf(`
root:x:0:root
bin:x:1:root,bin,daemon
daemon:x:2:root,bin,daemon
sys:x:3:root,bin,adm
adm:x:4:root,adm,daemon
tty:x:5:
disk:x:6:root,adm
lp:x:7:lp
mem:x:8:
kmem:x:9:
wheel:x:10:root
floppy:x:11:root
mail:x:12:mail
news:x:13:news
uucp:x:14:uucp
man:x:15:man
cron:x:16:cron
console:x:17:
audio:x:18:
cdrom:x:19:
dialout:x:20:root
ftp:x:21:
sshd:x:22:
input:x:23:
at:x:25:at
tape:x:26:root
video:x:27:root
netdev:x:28:
readproc:x:30:
squid:x:31:squid
xfs:x:33:xfs
kvm:x:34:kvm
games:x:35:
shadow:x:42:
cdrw:x:80:
www-data:x:82:
usb:x:85:
vpopmail:x:89:
users:x:100:games
ntp:x:123:
nofiles:x:200:
smmsp:x:209:smmsp
locate:x:245:
abuild:x:300:
utmp:x:406:
ping:x:999:
nogroup:x:65533:
nobody:x:65534:%s`, extra_group),
		},
		{
			"passwd",
			fmt.Sprintf(`
root:x:0:0:root:/root:/sbin/nologin
bin:x:1:1:bin:/bin:/sbin/nologin
daemon:x:2:2:daemon:/sbin:/sbin/nologin
adm:x:3:4:adm:/var/adm:/sbin/nologin
lp:x:4:7:lp:/var/spool/lpd:/sbin/nologin
sync:x:5:0:sync:/sbin:/bin/sync
shutdown:x:6:0:shutdown:/sbin:/sbin/shutdown
halt:x:7:0:halt:/sbin:/sbin/halt
mail:x:8:12:mail:/var/mail:/sbin/nologin
news:x:9:13:news:/usr/lib/news:/sbin/nologin
uucp:x:10:14:uucp:/var/spool/uucppublic:/sbin/nologin
operator:x:11:0:operator:/root:/sbin/nologin
man:x:13:15:man:/usr/man:/sbin/nologin
postmaster:x:14:12:postmaster:/var/mail:/sbin/nologin
cron:x:16:16:cron:/var/spool/cron:/sbin/nologin
ftp:x:21:21::/var/lib/ftp:/sbin/nologin
sshd:x:22:22:sshd:/dev/null:/sbin/nologin
at:x:25:25:at:/var/spool/cron/atjobs:/sbin/nologin
squid:x:31:31:Squid:/var/cache/squid:/sbin/nologin
xfs:x:33:33:X Font Server:/etc/X11/fs:/sbin/nologin
games:x:35:35:games:/usr/games:/sbin/nologin
cyrus:x:85:12::/usr/cyrus:/sbin/nologin
vpopmail:x:89:89::/var/vpopmail:/sbin/nologin
ntp:x:123:123:NTP:/var/empty:/sbin/nologin
smmsp:x:209:209:smmsp:/var/spool/mqueue:/sbin/nologin
guest:x:405:100:guest:/dev/null:/sbin/nologin
nobody:x:65534:65534:nobody:/:/sbin/nologin%s`, extra_user),
		},
	}
	for _, file := range files {
		hdr := &tar.Header{
			Name: file.Name,
			Mode: 0600,
			Size: int64(len(file.Body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			Logger.Error(err)
		}
		if _, err := tw.Write([]byte(file.Body)); err != nil {
			Logger.Error(err)
		}
	}
	if err := tw.Close(); err != nil {
		Logger.Error(err)
	}

	return io.ReadAll(&buf)
}
