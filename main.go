package main

import (
	"flag"
	"fmt"
	"github.com/hashicorp/memberlist"
	"golang.org/x/net/context"
	"io/ioutil"
	"log"
	"net"
	"strings"
	"time"
)

const (
	leaveTimeout = 2 * time.Second
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("gossipchat: ")

	port := flag.Int("port", 0, "port on which to listen")
	nick := flag.String("nick", "", "nickname to use on the chat")
	peersCSL := flag.String("peers", "", "comma separated list of addresses where peers can be found")
	flag.Parse()

	if *nick == "" {
		log.Fatal("you need to provice a -nick")
	}

	peers, err := parsePeers(*peersCSL)
	if err != nil {
		log.Fatalf("invalid value for flag -peers. %v", err)
	}

	var (
		msgc    <-chan []message
		memberc <-chan []string

		chatc = make(chan message, 1)
	)
	delegate, msgc := newDelegate(100, chatc)

	conf := memberlist.DefaultLANConfig()
	conf.LogOutput = ioutil.Discard
	conf.BindPort = *port
	conf.Name = *nick
	conf.Delegate = delegate
	conf.Events, memberc = newEvents()
	conf.Conflict = newConflicts()
	conf.PushPullInterval = time.Second
	mlist, err := memberlist.Create(conf)
	if err != nil {
		log.Fatalf("can't create memberlist: %v", err)
	}
	log.SetPrefix(mlist.LocalNode().Name + ": ")
	delegate.queue.NumNodes = mlist.NumMembers

	count, err := mlist.Join(peers)
	if err != nil {
		log.Printf("can't join peers: %v", err)
	}
	log.Printf("joined %d peers", count)

	log.Printf("we are reachable at %q", fmt.Sprintf("%s:%d",
		mlist.LocalNode().Addr, mlist.LocalNode().Port))
	name := mlist.LocalNode().Name

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer cancel()
		uichatc, err := runUI(ctx, name, memberc, msgc)
		if err != nil {
			log.Printf("can't start UI: %v", err)
			return
		}
		for chatmsg := range uichatc {
			select {
			case chatc <- chatmsg:
			case <-ctx.Done():
				return
			}
		}
		log.Printf("stopped UI")
	}()

	<-ctx.Done()

	if err := mlist.Leave(leaveTimeout); err != nil {
		log.Printf("could not announce leave: %v", err)
	}
	if err := mlist.Shutdown(); err != nil {
		log.Printf("could not shutdown cleanly: %v", err)
	}
}

func parsePeers(peersCSL string) ([]string, error) {
	if peersCSL == "" {
		return nil, nil
	}
	var peers []string
	for _, peer := range strings.Split(peersCSL, ",") {
		peer = strings.TrimSpace(peer)
		_, err := net.ResolveTCPAddr("tcp", peer)
		if err != nil {
			return nil, fmt.Errorf("can't resolve %q: %v", peer, err)
		}
		peers = append(peers, peer)
	}
	return peers, nil
}
