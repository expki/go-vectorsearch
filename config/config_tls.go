package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	mrand "math/rand"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/logger"
)

type ConfigTLS struct {
	mutex            *sync.RWMutex    `json:"-"`
	DomainNameServer []string         `json:"dns"`
	IP               []string         `json:"ip"`
	Certificates     []*ConfigTLSPath `json:"certificates"`
}

// Configurate initialize the tls configuration.
func (t *ConfigTLS) Configurate() error {
	if t.mutex == nil {
		t.mutex = &sync.RWMutex{}
	}
	err := t.reloadCertificates()
	if err != nil {
		return errors.Join(errors.New("could not load certificates"), err)
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		for {
			<-ticker.C
			err := t.reloadCertificates()
			if err != nil {
				logger.Sugar().Errorf("could not reload certificates: %v", err)
			}
		}
	}()
	err = t.generateMissingCertificates()
	if err != nil {
		return errors.Join(errors.New("could not generate missing certificates"), err)
	}
	return nil
}

func (t *ConfigTLS) getDNS() []string {
	list := t.DomainNameServer
	if len(list) == 0 {
		list = getAllDNSServers()
	}
	return list
}

func (t *ConfigTLS) getIP() []net.IP {
	list := make([]net.IP, 0, len(t.IP))
	for _, address := range t.IP {
		list = append(list, net.ParseIP(address))
	}
	if len(list) == 0 {
		list = getAllIPAddresses()
	}
	return list
}

// GetCertificate returns the first client supported certificate.
func (t *ConfigTLS) GetCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	// return first certificate if no sni supplied
	if clientHello == nil {
		tlsPath := t.Certificates[0]
		tlsPath.mutex.RLock()
		certificate := tlsPath.certificate
		tlsPath.mutex.RUnlock()
		return &certificate, nil
	}

	// find sni supported certificate
	for _, tlsPath := range t.Certificates {
		tlsPath.mutex.RLock()
		certificate := tlsPath.certificate
		tlsPath.mutex.RUnlock()
		err := clientHello.SupportsCertificate(&certificate)
		if err == nil {
			return &certificate, nil
		}
	}

	// return first certificate if no supported certificate found
	tlsPath := t.Certificates[0]
	tlsPath.mutex.RLock()
	certificate := tlsPath.certificate
	tlsPath.mutex.Unlock()
	return &certificate, nil
}

// reloadCertificates reloads all certificates.
func (t *ConfigTLS) reloadCertificates() error {
	// reload individual certificates
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	for _, tlsPath := range t.Certificates {
		err := tlsPath.reloadCertificate()
		if err != nil {
			return err
		}
	}
	return nil
}

// generateMissingCertificates generates missing certificates.
func (t *ConfigTLS) generateMissingCertificates() error {
	var hasRSA, hasECDSA bool = false, false
	t.mutex.RLock()
	for _, tlsPath := range t.Certificates {
		tlsPath.mutex.RLock()
		switch tlsPath.algorithm {
		case algorithm_rsa:
			hasRSA = true
		case algorithm_ecdsa:
			hasECDSA = true
		}
		tlsPath.mutex.RUnlock()
	}
	t.mutex.RUnlock()
	var certificate tls.Certificate
	var ecdsaErr, rsaErr error
	if !hasECDSA {
		certificate, ecdsaErr = generateCertificateECDSA(t.getDNS(), t.getIP())
		if ecdsaErr == nil {
			t.mutex.Lock()
			t.Certificates = append(t.Certificates, &ConfigTLSPath{
				algorithm:   algorithm_ecdsa,
				certificate: certificate,
			})
			t.mutex.Unlock()
		}
	}
	if !hasRSA {
		certificate, rsaErr = generateCertificateRSA(t.getDNS(), t.getIP())
		if rsaErr == nil {
			t.mutex.Lock()
			t.Certificates = append(t.Certificates, &ConfigTLSPath{
				algorithm:   algorithm_rsa,
				certificate: certificate,
			})
			t.mutex.Unlock()
		}
	}
	var listErr []error
	if ecdsaErr != nil {
		listErr = append(listErr, errors.Join(errors.New("ecdsa"), ecdsaErr))
	}
	if rsaErr != nil {
		listErr = append(listErr, errors.Join(errors.New("rsa"), rsaErr))
	}
	if len(listErr) > 0 {
		return errors.Join(errors.New("failed to generate certificates"), errors.Join(listErr...))
	}
	return nil
}

// generateCertificateECDSA generates a new ECDSA certificate.
func generateCertificateECDSA(dns []string, ip []net.IP) (certificate tls.Certificate, err error) {
	// gernerate private key
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return certificate, errors.Join(errors.New("could not generate ecdsa key"), err)
	}
	certificate.PrivateKey = key

	// generate SKI
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return certificate, errors.Join(errors.New("could not marshal public key"), err)
	}
	ski := sha1.Sum(pubKeyBytes)

	// sign certificate
	leaf := &x509.Certificate{
		SerialNumber:          big.NewInt(mrand.Int63()),
		Subject:               pkix.Name{CommonName: "localhost", Organization: []string{"vdh.dev"}},
		PublicKey:             key.Public(),
		PublicKeyAlgorithm:    x509.ECDSA,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:              dns,
		IPAddresses:           ip,
		IsCA:                  true,
		BasicConstraintsValid: true,
		SignatureAlgorithm:    x509.ECDSAWithSHA384,
		SubjectKeyId:          ski[:],
		AuthorityKeyId:        ski[:],
	}
	leafDer, err := x509.CreateCertificate(rand.Reader, leaf, leaf, key.Public(), key)
	if err != nil {
		return certificate, errors.Join(errors.New("could not create ecdsa certificate"), err)
	}
	certificate.Certificate = [][]byte{leafDer}

	// parse certificate
	certificate.Leaf, err = x509.ParseCertificate(leafDer)
	if err != nil {
		return certificate, errors.Join(errors.New("could not parse ecdsa certificate"), err)
	}

	return certificate, nil
}

// generateCertificateRSA generates a new RSA certificate.
func generateCertificateRSA(dns []string, ip []net.IP) (certificate tls.Certificate, err error) {
	// gernerate private key
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return certificate, errors.Join(errors.New("could not generate rsa key"), err)
	}
	certificate.PrivateKey = key

	// generate SKI
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return certificate, errors.Join(errors.New("could not marshal public key"), err)
	}
	ski := sha1.Sum(pubKeyBytes)

	// sign certificate
	leaf := &x509.Certificate{
		SerialNumber:          big.NewInt(mrand.Int63()),
		Subject:               pkix.Name{CommonName: "localhost", Organization: []string{"vdh.dev"}},
		PublicKey:             key.Public(),
		PublicKeyAlgorithm:    x509.RSA,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyAgreement | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:              dns,
		IPAddresses:           ip,
		IsCA:                  true,
		BasicConstraintsValid: true,
		SignatureAlgorithm:    x509.SHA384WithRSA,
		SubjectKeyId:          ski[:],
		AuthorityKeyId:        ski[:],
	}
	leafDer, err := x509.CreateCertificate(rand.Reader, leaf, leaf, key.Public(), key)
	if err != nil {
		return tls.Certificate{}, errors.Join(errors.New("could not create rsa certificate"), err)
	}
	certificate.Certificate = [][]byte{leafDer}

	// parse certificate
	certificate.Leaf, err = x509.ParseCertificate(leafDer)
	if err != nil {
		return certificate, errors.Join(errors.New("could not parse rsa certificate"), err)
	}

	return certificate, nil
}

// getAllIPAddresses returns all ip addresses of the system.
func getAllIPAddresses() []net.IP {
	// get all interfaces
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return []net.IP{net.ParseIP("localhost"), net.ParseIP("::1")}
	}

	// get all ip addresses
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil {
			ips = append(ips, ip)
		}
	}

	// return list
	return ips
}

// getAllDNSServers returns all dns servers of the system.
func getAllDNSServers() (dnsServerlist []string) {
	// add localhost
	dnsServerlist = append(dnsServerlist, "localhost")

	// add hostname
	hostname, err := os.Hostname()
	if err != nil {
		return
	}
	dnsServerlist = append([]string{hostname}, dnsServerlist...)

	// add fqdn
	addrs, err := net.LookupIP(hostname)
	if err != nil {
		return
	}
	for _, addr := range addrs {
		names, err := net.LookupAddr(addr.String())
		if err != nil {
			continue
		}
		for _, name := range names {
			dnsServerlist = append([]string{strings.TrimSuffix(name, ".")}, dnsServerlist...)
		}
	}

	// remove duplicates
	uniqueList := make([]string, 0, len(dnsServerlist))
	for _, dnsServer := range dnsServerlist {
		if !slices.Contains(uniqueList, dnsServer) {
			uniqueList = append(uniqueList, dnsServer)
		}
	}
	dnsServerlist = uniqueList

	// return list
	return
}
