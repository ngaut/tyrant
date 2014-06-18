package zk

import (
	"errors"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	log "github.com/ngaut/logging"
	"github.com/samuel/go-zookeeper/zk"
)

var (
	// error
	ErrNoChild      = errors.New("zk: children is nil")
	ErrNodeNotExist = errors.New("zk: node not exist")
)

// Connect connect to zookeeper, and start a goroutine log the event.
func Connect(addr []string, timeout time.Duration) (*zk.Conn, error) {
	conn, session, err := zk.Connect(addr, timeout)
	if err != nil {
		log.Errorf(`zk.Connect("%v", %d) error(%v)`, addr, timeout, err)
		return nil, err
	}
	go func() {
		for {
			event := <-session
			log.Infof("zookeeper get a event: %s", event.State.String())
		}
	}()

	return conn, nil
}

// Create create zookeeper path, if path exists ignore error
func Create(conn *zk.Conn, fpath string) error {
	// create zk root path
	tpath := ""
	for _, str := range strings.Split(fpath, "/")[1:] {
		tpath = path.Join(tpath, "/", str)
		log.Infof(`create zookeeper path: "%s"`, tpath)
		_, err := conn.Create(tpath, []byte(""), 0, zk.WorldACL(zk.PermAll))
		if err != nil {
			if err == zk.ErrNodeExists {
				log.Warningf(`zk.create("%s") exists`, tpath)
			} else {
				log.Errorf(`zk.create("%s") error(%v)`, tpath, err)
				return err
			}
		}
	}

	return nil
}

// RegisterTmp create a ephemeral node, and watch it, if node droped then send a SIGQUIT to self.
func RegisterTemp(conn *zk.Conn, fpath, data string) error {
	log.Debug(fpath)
	tpath, err := conn.Create(path.Join(fpath), []byte(data), zk.FlagEphemeral /*|zk.FlagSequence*/, zk.WorldACL(zk.PermAll))
	if err != nil {
		log.Errorf(`conn.Create("%s", "%s", zk.FlagEphemeral|zk.FlagSequence) error(%v)`, fpath, data, err)
		return err
	}

	log.Infof("create a zookeeper node:%s", tpath)
	// watch self
	go func() {
		for {
			log.Info("watch", tpath)
			exist, _, watch, err := conn.ExistsW(tpath)
			if err != nil {
				log.Errorf(`zk path: "%s" set watch failed, error: , kill itself`, tpath, err)
				killSelf()
				return
			}
			if !exist {
				log.Errorf(`zk path: "%s" not exist, kill itself`, tpath)
				killSelf()
				return
			}
			event := <-watch
			log.Infof(`zk path: "%s" receive a event %v`, tpath, event)
		}
	}()

	return nil
}

// GetNodesW get all child from zk path with a watch.
func GetNodesW(conn *zk.Conn, path string) ([]string, <-chan zk.Event, error) {
	nodes, stat, watch, err := conn.ChildrenW(path)
	if err != nil {
		if err == zk.ErrNoNode {
			return nil, nil, ErrNodeNotExist
		}
		log.Errorf(`zk.ChildrenW("%s") error(%v)`, path, err)
		return nil, nil, err
	}

	if stat == nil {
		return nil, nil, ErrNodeNotExist
	}

	if len(nodes) == 0 {
		return nil, nil, ErrNoChild
	}

	return nodes, watch, nil
}

// GetNodes get all child from zk path.
func GetNodes(conn *zk.Conn, path string) ([]string, error) {
	nodes, stat, err := conn.Children(path)
	if err != nil {
		if err == zk.ErrNoNode {
			return nil, ErrNodeNotExist
		}
		log.Errorf(`zk.Children("%s") error(%v)`, path, err)
		return nil, err
	}

	if stat == nil {
		return nil, ErrNodeNotExist
	}

	if len(nodes) == 0 {
		return nil, ErrNoChild
	}

	return nodes, nil
}

// killSelf send a SIGQUIT to self.
func killSelf() {
	if err := syscall.Kill(os.Getpid(), syscall.SIGQUIT); err != nil {
		log.Errorf("syscall.Kill(%d, SIGQUIT) error(%v)", os.Getpid(), err)
	}
}