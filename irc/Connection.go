package irc

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

const TRIM = " \r\n"

type IRC struct {
	Tx     chan Event // transmit
	Rx     chan Event // receive
	server string
}

type Event interface {
	Send(conn net.Conn) error
}

type EConnect struct{}

func (ec *EConnect) Send(conn net.Conn) error {
	panic("The EConnect type cannot be sent")
	return nil
}

type EDisconnect struct{}

func (ec *EDisconnect) Send(conn net.Conn) error {
	panic("The EDisconnect type cannot be sent")
	return nil
}

type Line struct {
	Prefix    string
	Command   string
	Arguments []string
	Suffix    string
}

func (el *Line) Send(conn net.Conn) error {
	line := el.Raw();
	log.Printf("Sending Line: %s", line);
	conn.Write([]byte(line));
	return nil
}

func (el *Line) Raw() (s string) {
	if len(el.Prefix) > 0 {
		s = ":" + el.Prefix + " "
	}
	
	s = s + el.Command + " ";
	
	if len(el.Arguments) > 0 {
		s = s + strings.Join(el.Arguments, " ") + " ";
	}
	
	if len(el.Suffix) > 0 {
		s = s + ":" + el.Suffix;
	}

	s = s + "\r\n";
	return
}

func NewLine(line string) (*Line, error) {
	line = strings.Trim(line, TRIM)

	result := &Line{
		Prefix:    "",
		Command:   "",
		Arguments: []string{},
		Suffix:    "",
	}

	prefixEnd := -1
	trailingStart := len(line)

	if trailingStart == 0 {
		return nil, errors.New("Line is 0 characters long. This is too short")
	}

	//determine the prefix if one is present
	if string(line[0]) == ":" {
		if i := strings.Index(line, " "); i != -1 {
			prefixEnd = i
			result.Prefix = line[1:i]
		}
		// else { no prefix is present. no problemo }
	}

	//determine if a suffix is present
	if i := strings.Index(line, " :"); i != -1 {
		trailingStart = i
		result.Suffix = line[i+2:]
	}
	// else { no suffix is present. no problemo }

	params_str := line[prefixEnd+1 : trailingStart]

	params := strings.Split(params_str, " ")

	if len(params) == 0 {
		return nil, errors.New("There is no command")
	}

	result.Command = params[0]

	if len(params) > 1 {
		result.Arguments = params[1:]
	}

	return result, nil
}

func NewIRC(server string) *IRC {
	i := &IRC{
		Tx:     nil,
		Rx:     make(chan Event),
		server: server,
	}
	go i.Run()
	return i
}

func (i *IRC) PingHandler(e Event) bool {
	handled := false

	switch l := e.(type) {
	case *Line:
		switch l.Command {
		case "PING":
			i.Tx <- &Line{
				Command: "PONG",
				Arguments: l.Arguments,
			}
		}
	}

	return handled
}

func (i *IRC) Run() {
	var sock net.Conn = nil
	for {
		if sock == nil {
			log.Printf("Dialing...");
			c, err := i.connect()
			if err != nil {
				log.Printf("Could not connect. Retrying in 10s: %s\n", err)
				time.Sleep(10 * time.Second)
				continue
			}
			log.Printf("Dialed");
			sock = c
			i.Tx = make(chan Event)
			i.Rx <- &EConnect{}
		}
		go i.writeRoutine(sock)
		bufio := bufio.NewReader(sock)
		for {
			sock.SetReadDeadline(time.Now().Add(time.Second * 60))
			line, err := bufio.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					log.Printf("Got EOF. read thread restarting")
					close(i.Tx)
					sock = nil
					break
				}
				log.Printf("Reading error: %s\n", err)
				continue
			}
			iline, err := NewLine(line)
			if err != nil {
				log.Printf("Line parsing error: %s\n", err)
				continue
			}
			i.Rx <- iline
		}
		i.Rx <- &EDisconnect{}
	}
}

func (i *IRC) writeRoutine(conn net.Conn) {
	for w := range i.Tx {
		if e := w.Send(conn); e != nil {
			if e == io.EOF {
				log.Printf("Got EOF in write thread. write thread exiting")
				return
			}
			log.Printf("Sending error: %s\n", e)
			continue
		}
	}
	log.Printf("Write thread terminating")
}

// TODO: use a net.Dialer -- should provide us with proxy support
func (i *IRC) connect() (net.Conn, error) {
	return net.Dial("tcp", i.server)
}
