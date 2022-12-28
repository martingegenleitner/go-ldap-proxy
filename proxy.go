// Stolen from https://github.com/nmcclain/ldap/blob/master/examples/proxy.go

package main

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/joho/godotenv"
	"github.com/nmcclain/ldap"
	log "github.com/sirupsen/logrus"
)

type ldapHandler struct {
	sessions   map[string]session
	lock       *sync.Mutex
	ldapServer string
	ldapPort   int
}

func init() {
	// Log as JSON instead of the default ASCII formatter.
	//log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.DebugLevel)
}

// /////////// Run a simple LDAP proxy
func main() {

	err := godotenv.Load()
	if err != nil {
		log.Info("Failed loading .env file.", err.Error())
	}

	s := ldap.NewServer()

	upstreamPort, err := strconv.Atoi(getOSEnv("UPSTREAM_LDAP_SERVER_PORT", "389"))
	if err != nil {
		log.Fatal("Failed to parse UPSTREAM_LDAP_SERVER_PORT. %s given", getOSEnv("UPSTREAM_LDAP_SERVER_PORT", "389"))
	}
	handler := ldapHandler{
		sessions:   make(map[string]session),
		ldapServer: os.Getenv("UPSTREAM_LDAP_SERVER_HOST"),
		ldapPort:   upstreamPort,
	}
	s.BindFunc("", handler)
	s.SearchFunc("", handler)
	s.CloseFunc("", handler)

	servicePort, err := strconv.Atoi(getOSEnv("LISTENING_PORT", "8000"))
	if err != nil {
		log.Fatal("Failed to parse LISTENING_PORT. %s given", getOSEnv("LISTENING_PORT", "8000"))
	}

	// start the server
	log.Printf("Starting server on %s:%d ...\n", "localhost", servicePort)
	if err := s.ListenAndServe("0.0.0.0:" + getOSEnv("LISTENING_PORT", "8000")); err != nil {
		log.Fatal("LDAP Server Failed: %s\n", err.Error())
	}
}

// ///////////
type session struct {
	id   string
	c    net.Conn
	ldap *ldap.Conn
}

func (h ldapHandler) getSession(conn net.Conn) (session, error) {
	id := connID(conn)
	h.lock.Lock()
	s, ok := h.sessions[id] // use server connection if it exists
	h.lock.Unlock()
	if !ok { // open a new server connection if not
		l, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", h.ldapServer, h.ldapPort))
		if err != nil {
			return session{}, err
		}
		s = session{id: id, c: conn, ldap: l}
		h.lock.Lock()
		h.sessions[s.id] = s
		h.lock.Unlock()
	}
	return s, nil
}

// ///////////
func (h ldapHandler) Bind(bindDN, bindSimplePw string, conn net.Conn) (ldap.LDAPResultCode, error) {
	s, err := h.getSession(conn)
	if err != nil {
		return ldap.LDAPResultOperationsError, err
	}

	log.Printf("BIND from %s received. Processing authentication ...\n", bindDN)

	if err := s.ldap.Bind(bindDN, bindSimplePw); err != nil {
		return ldap.LDAPResultOperationsError, err
	}
	return ldap.LDAPResultSuccess, nil
}

// ///////////
func (h ldapHandler) Search(boundDN string, searchReq ldap.SearchRequest, conn net.Conn) (ldap.ServerSearchResult, error) {
	s, err := h.getSession(conn)
	if err != nil {
		return ldap.ServerSearchResult{ResultCode: ldap.LDAPResultOperationsError}, nil
	}
	search := ldap.NewSearchRequest(
		searchReq.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		searchReq.Filter,
		searchReq.Attributes,
		nil)
	sr, err := s.ldap.Search(search)
	if err != nil {
		return ldap.ServerSearchResult{}, err
	}
	//log.Printf("P: Search OK: %s -> num of entries = %d\n", search.Filter, len(sr.Entries))
	return ldap.ServerSearchResult{Entries: sr.Entries, Referrals: []string{}, Controls: []ldap.Control{}, ResultCode: ldap.LDAPResultSuccess}, nil
}
func (h ldapHandler) Close(boundDN string, conn net.Conn) error {
	conn.Close() // close connection to the server when then client is closed
	h.lock.Lock()
	defer h.lock.Unlock()
	delete(h.sessions, connID(conn))
	return nil
}
func connID(conn net.Conn) string {
	h := sha256.New()
	h.Write([]byte(conn.LocalAddr().String() + conn.RemoteAddr().String()))
	sha := fmt.Sprintf("% x", h.Sum(nil))
	return string(sha)
}

func getOSEnv(key string, defaultValue string) string {
	valueFromOS := os.Getenv(key)
	if valueFromOS == "" {
		return defaultValue
	}
	return valueFromOS
}
