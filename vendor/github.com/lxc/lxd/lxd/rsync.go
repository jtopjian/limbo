package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"

	"github.com/gorilla/websocket"
	"github.com/pborman/uuid"

	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/logger"
)

// rsyncCopy copies a directory using rsync (with the --devices option).
func rsyncLocalCopy(source string, dest string, bwlimit string) (string, error) {
	err := os.MkdirAll(dest, 0755)
	if err != nil {
		return "", err
	}

	rsyncVerbosity := "-q"
	if debug {
		rsyncVerbosity = "-vi"
	}

	if bwlimit == "" {
		bwlimit = "0"
	}

	return shared.RunCommand("rsync",
		"-a",
		"-HAX",
		"--sparse",
		"--devices",
		"--delete",
		"--checksum",
		"--numeric-ids",
		"--bwlimit", bwlimit,
		rsyncVerbosity,
		shared.AddSlash(source),
		dest)
}

func rsyncSendSetup(name string, path string, bwlimit string) (*exec.Cmd, net.Conn, io.ReadCloser, error) {
	/*
	 * The way rsync works, it invokes a subprocess that does the actual
	 * talking (given to it by a -E argument). Since there isn't an easy
	 * way for us to capture this process' stdin/stdout, we just use netcat
	 * and write to/from a unix socket.
	 *
	 * In principle we don't need this socket. It seems to me that some
	 * clever invocation of rsync --server --sender and usage of that
	 * process' stdin/stdout could work around the need for this socket,
	 * but I couldn't get it to work. Another option would be to look at
	 * the spawned process' first child and read/write from its
	 * stdin/stdout, but that also seemed messy. In any case, this seems to
	 * work just fine.
	 */
	auds := fmt.Sprintf("@lxd/%s", uuid.NewRandom().String())
	// We simply copy a part of the uuid if it's longer than the allowed
	// maximum. That should be safe enough for our purposes.
	if len(auds) > shared.ABSTRACT_UNIX_SOCK_LEN-1 {
		auds = auds[:shared.ABSTRACT_UNIX_SOCK_LEN-1]
	}
	l, err := net.Listen("unix", auds)
	if err != nil {
		return nil, nil, nil, err
	}

	/*
	 * Here, the path /tmp/foo is ignored. Since we specify localhost,
	 * rsync thinks we are syncing to a remote host (in this case, the
	 * other end of the lxd websocket), and so the path specified on the
	 * --server instance of rsync takes precedence.
	 *
	 * Additionally, we use sh -c instead of just calling nc directly
	 * because rsync passes a whole bunch of arguments to the wrapper
	 * command (i.e. the command to run on --server). However, we're
	 * hardcoding that at the other end, so we can just ignore it.
	 */
	rsyncCmd := fmt.Sprintf("sh -c \"%s netcat %s %s\"", execPath, auds, name)
	if bwlimit == "" {
		bwlimit = "0"
	}

	cmd := exec.Command("rsync",
		"-arvP",
		"--devices",
		"--numeric-ids",
		"--partial",
		"--sparse",
		path,
		"localhost:/tmp/foo",
		"-e",
		rsyncCmd,
		"--bwlimit",
		bwlimit)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}

	conn, err := l.Accept()
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, nil, nil, err
	}
	l.Close()

	return cmd, conn, stderr, nil
}

// RsyncSend sets up the sending half of an rsync, to recursively send the
// directory pointed to by path over the websocket.
func RsyncSend(name string, path string, conn *websocket.Conn, readWrapper func(io.ReadCloser) io.ReadCloser, bwlimit string) error {
	cmd, dataSocket, stderr, err := rsyncSendSetup(name, path, bwlimit)
	if err != nil {
		return err
	}

	if dataSocket != nil {
		defer dataSocket.Close()
	}

	readPipe := io.ReadCloser(dataSocket)
	if readWrapper != nil {
		readPipe = readWrapper(dataSocket)
	}

	readDone, writeDone := shared.WebsocketMirror(conn, dataSocket, readPipe, nil, nil)

	output, err := ioutil.ReadAll(stderr)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return err
	}

	err = cmd.Wait()
	if err != nil {
		logger.Errorf("Rsync send failed: %s: %s: %s", path, err, string(output))
	}

	<-readDone
	<-writeDone

	return err
}

// RsyncRecv sets up the receiving half of the websocket to rsync (the other
// half set up by RsyncSend), putting the contents in the directory specified
// by path.
func RsyncRecv(path string, conn *websocket.Conn, writeWrapper func(io.WriteCloser) io.WriteCloser) error {
	cmd := exec.Command("rsync",
		"--server",
		"-vlogDtpre.iLsfx",
		"--numeric-ids",
		"--devices",
		"--partial",
		"--sparse",
		".",
		path)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	writePipe := io.WriteCloser(stdin)
	if writeWrapper != nil {
		writePipe = writeWrapper(stdin)
	}

	readDone, writeDone := shared.WebsocketMirror(conn, writePipe, stdout, nil, nil)
	output, err := ioutil.ReadAll(stderr)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return err
	}

	err = cmd.Wait()
	if err != nil {
		logger.Errorf("Rsync receive failed: %s: %s: %s", path, err, string(output))
	}

	<-readDone
	<-writeDone

	return err
}
