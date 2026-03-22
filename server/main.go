package localserver

import sftpserver "blackout-saver/server/sftp"

func main() {
	sftpserver.ListenSSH()
}
