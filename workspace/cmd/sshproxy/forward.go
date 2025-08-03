package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"

	authType "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
)

func forward(ctx context.Context, conn net.Conn) {
	log.Debugf("connected: %v", conn.RemoteAddr())
	defer log.Debugf("disconnected: %v", conn.RemoteAddr())
	defer conn.Close()

	// The context should be canceled in 1 week.
	// This is the longest session we can tolerate.
	// Upon canceling, all channels should be closed.
	ctx, cancel := context.WithTimeout(ctx, 7*24*time.Hour)
	defer cancel()

	// Detect and close idle connections.
	conn = Idle(ctx, conn, 10*time.Minute)

	// Accept the connection from the user.
	var config *ssh.ServerConfig
	{
		// Set the handshake timeout.
		_, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		config = makeServerConfig(ctx, conn)
	}
	userConn, userChans, userReqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		log.Debugf("NewServerConn: %v", err)
		return
	}
	defer userConn.Close()

	podIP, err := GetPodIP(*flagServer, userConn.User(), consts.KeyInspectTypeNegative, nil)
	if err != nil {
		log.WithError(err).Error("failed to GetPodIP")
		return
	}

	log.Debugf("authenticated: %v", conn.RemoteAddr())
	forwardSSH(ctx, userConn, userChans, userReqs, "ais", "root", podIP)
}

func makeServerConfig(ctx context.Context, rawConn net.Conn) *ssh.ServerConfig {
	config := &ssh.ServerConfig{
		NoClientAuth: *flagNoAuth,
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if *flagNoAuth {
				return nil, nil
			}

			if _, err := GetPodIP(*flagServer, conn.User(), consts.KeyInspectTypePositive, key); err != nil {
				log.WithError(err).Error("failed to GetPodIP")
				return nil, fmt.Errorf("invalid login: %s", conn.User())
			}

			return nil, nil
		},
	}
	config.AddHostKey(serverKey)
	return config
}

func GetPodIP(server, user, inspect string, key ssh.PublicKey) (string, error) {
	// 其中 user 为 workspaceID.userID
	params := strings.Split(user, ".")
	if len(params) < 1 {
		return "", fmt.Errorf("failed to get user info")
	}

	url := fmt.Sprintf("%s/api/v1/workspaces/%s", server, params[0])

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	if len(params) > 1 {
		req.Header.Add(authType.AISSubjectHeader, params[1])
	} else {
		req.Header.Add(authType.AISSubjectHeader, "admin")
	}
	req.Header.Add(authType.AISProjectHeader, "admin")
	req.Header.Add(authType.AISTenantHeader, "admin")

	req.Header.Add(consts.KeyInspect, inspect)
	logP := log.WithField(consts.KeyInspect, inspect)
	if consts.KeyInspectTypes[inspect] {
		req.Header.Add(consts.KeyType, key.Type())
		req.Header.Add(consts.KeyMarshal, base64.StdEncoding.EncodeToString(key.Marshal()))
		logP = logP.WithField(consts.KeyType, key.Type()).WithField(consts.KeyMarshal, base64.StdEncoding.EncodeToString(key.Marshal()))
	}
	logP.Warn("get pod ip requset")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get pods ip")
	}

	var successResult struct {
		Data struct {
			Instance struct {
				PodIP string `json:"podIP"`
			} `json:"instance"`
		} `json:"data"`
	}

	fmt.Println(string(body))
	if err := json.Unmarshal(body, &successResult); err != nil {
		return "", err
	}

	return successResult.Data.Instance.PodIP, nil
}
