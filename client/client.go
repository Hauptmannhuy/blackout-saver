package client

import "golang.org/x/crypto/ssh"

type Uploader interface {
	Upload()
}
type Client struct {
	Uploader string
}

type SFTPUploader struct {
}

func abc() {
	client, err := ssh.Dial("tcp")

}
