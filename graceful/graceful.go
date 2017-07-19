package graceful

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

var DefaultShutdownTimeout = time.Second * 60

type GracefulServer struct {
	server   *http.Server
	listener net.Listener
	log      *logrus.Entry

	URL             string
	ShutdownTimeout time.Duration
}

func NewGracefulServer(handler http.Handler, log *logrus.Entry) *GracefulServer {
	return &GracefulServer{
		server:          &http.Server{Handler: handler},
		log:             log,
		listener:        nil,
		ShutdownTimeout: DefaultShutdownTimeout,
	}
}

func (svr *GracefulServer) Bind(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	svr.URL = "http://" + l.Addr().String()
	svr.listener = l
	return nil
}

func (svr *GracefulServer) Listen() error {
	go svr.listenForShutdownSignal()
	return svr.server.Serve(svr.listener)
}

func (svr *GracefulServer) listenForShutdownSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	sig := <-c
	svr.log.Infof("Triggering shutdown from signal %s", sig)

	shutErr := svr.Close()
	if shutErr == context.DeadlineExceeded {
		svr.log.WithError(shutErr).Warnf("Forcing a shutdown after waiting %s", svr.ShutdownTimeout.String())
		shutErr = svr.server.Close()
	}

	if shutErr != nil {
		svr.log.WithError(shutErr).Warnf("Error while shutting down")
	}

}

func (svr *GracefulServer) ListenAndServe(addr string) error {
	if svr.listener != nil {
		return errors.New("The listener has already started, don't call Bind first")
	}
	if err := svr.Bind(addr); err != nil {
		return err
	}

	return svr.Listen()
}

func (svr *GracefulServer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), svr.ShutdownTimeout)
	defer cancel()

	svr.log.Infof("Triggering shutdown, in at most %s ", svr.ShutdownTimeout.String())
	return svr.server.Shutdown(ctx)
}
