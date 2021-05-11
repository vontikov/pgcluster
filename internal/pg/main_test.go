package pg

import (
	"flag"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/vontikov/pgcluster/internal/docker"
	"github.com/vontikov/pgcluster/internal/logging"
	"github.com/vontikov/pgcluster/internal/util"
)

const (
	imageName         = "postgres"
	imageMajorVersion = 13
	imageMinorVersion = 2
	targetPort        = DefaultPort
	publishedPort     = DefaultPort
)

var image = fmt.Sprintf("%s:%d.%d", imageName, imageMajorVersion, imageMinorVersion)

var logger = func() logging.Logger {
	logging.SetLevel("debug")
	return logging.NewLogger("pg-test")
}()

var containerID string

func setup(cli *docker.Client) {
	env := []string{
		"POSTGRES_HOST_AUTH_METHOD=trust",
	}
	hostIP := "0.0.0.0"
	ports := [][]int{{publishedPort, targetPort}}
	containerName := "pg_test"
	network := "default"

	var err error

	err = cli.ImagePull(image)
	if err != nil {
		panic(err)
	}

	options := docker.ContainerOptions{
		Image:   image,
		Env:     env,
		HostIP:  hostIP,
		Ports:   ports,
		Name:    containerName,
		Network: network,
	}
	containerID, err = cli.ContainerCreate(options)
	util.PanicOnError(err)

	err = cli.ContainerStart(containerID)
	util.PanicOnError(err)
}

func shutdown(cli *docker.Client) {
	err := cli.ContainerStop(containerID)
	util.PanicOnError(err)

	err = cli.ContainerRemove(containerID)
	util.PanicOnError(err)
}

func TestA(t *testing.T) {
	log.Println("TestA running")
}

func TestB(t *testing.T) {
	log.Println("TestB running")
}

func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		logger.Info("skipping in short mode")
		return
	}

	cli, err := docker.New()
	util.PanicOnError(err)

	setup(cli)
	code := m.Run()
	shutdown(cli)
	os.Exit(code)
}
