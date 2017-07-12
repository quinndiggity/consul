package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/armon/circbuf"
)

// DockerClient is a simplified client for the Docker Engine API
// to execute the health checks and avoid significant dependencies.
// It also consumes all data returned from the Docker API through
// a ring buffer with a fixed limit to avoid excessive resource
// consumption.
type DockerClient struct {
	network string
	addr    string
	baseurl string
	maxbuf  int64
	client  *http.Client
}

func NewDockerClient(host string, maxbuf int64) (*DockerClient, error) {
	if host == "" {
		host = DefaultDockerHost
	}
	p := strings.SplitN(host, "://", 2)
	if len(p) == 1 {
		return nil, fmt.Errorf("invalid docker host: %s", host)
	}
	network, addr := p[0], p[1]
	basepath := "http://" + addr
	if network == "unix" {
		basepath = "http://unix"
	}
	client := &http.Client{}
	if network == "unix" {
		client.Transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		}
	}
	return &DockerClient{network, addr, basepath, maxbuf, client}, nil
}

func (c *DockerClient) call(method, uri string, okStatus int, v interface{}) (*circbuf.Buffer, error) {
	urlstr := c.baseurl + uri
	req, err := http.NewRequest(method, urlstr, nil)
	if err != nil {
		return nil, err
	}

	if v != nil {
		var b bytes.Buffer
		if err := json.NewEncoder(&b).Encode(v); err != nil {
			return nil, err
		}
		req.Body = ioutil.NopCloser(&b)
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := circbuf.NewBuffer(c.maxbuf)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(b, resp.Body)
	if err == nil && resp.StatusCode != okStatus {
		err = fmt.Errorf("bad status code: %s %d %s", urlstr, resp.StatusCode, b)
	}
	return b, err
}

func (c *DockerClient) CreateExec(containerID string, cmd []string) (string, error) {
	data := struct {
		AttachStdin  bool
		AttachStdout bool
		AttachStderr bool
		Tty          bool
		Cmd          []string
	}{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          cmd,
	}

	uri := fmt.Sprintf("/containers/%s/exec", containerID)
	b, err := c.call("POST", uri, http.StatusCreated, data)
	if err != nil {
		return "", fmt.Errorf("create exec: %v", err)
	}

	var resp struct{ Id string }
	if err = json.NewDecoder(bytes.NewReader(b.Bytes())).Decode(&resp); err != nil {
		return "", fmt.Errorf("error in create exec: %v", err)
	}

	return resp.Id, nil
}

func (c *DockerClient) StartExec(execID string) (*circbuf.Buffer, error) {
	data := struct{ Detach, Tty bool }{Detach: false, Tty: true}
	uri := fmt.Sprintf("/exec/%s/start", execID)
	b, err := c.call("POST", uri, http.StatusOK, data)
	if err != nil {
		return nil, fmt.Errorf("error in exec start: %v %s", err, b)
	}
	return b, nil
}

func (c *DockerClient) InspectExec(execID string) (int, error) {
	uri := fmt.Sprintf("/exec/%s/json", execID)
	b, err := c.call("GET", uri, http.StatusOK, nil)
	if err != nil {
		return 0, fmt.Errorf("error in exec inspect: %v %s", err, b)
	}
	var resp struct{ ExitCode int }
	if err := json.NewDecoder(bytes.NewReader(b.Bytes())).Decode(&resp); err != nil {
		return 0, fmt.Errorf("error in exec inspect: %v %s", err, b)
	}
	return resp.ExitCode, nil
}
