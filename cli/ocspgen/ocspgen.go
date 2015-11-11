// Package ocspgen implements the ocspgen command.
package ocspgen

import (
	"encoding/base64"
	"fmt"
	"time"

	"database/sql"

	"github.com/cloudflare/cfssl/certdb"
	"github.com/cloudflare/cfssl/cli"
	"github.com/cloudflare/cfssl/helpers"
	"github.com/cloudflare/cfssl/log"
	"github.com/cloudflare/cfssl/ocsp"
)

// Usage text of 'cfssl ocspgen'
var ocspgenUsageText = `cfssl ocspgen -- generates a series of concatenated OCSP responses
for use with ocspserve from all unexpired certs in the cert store

Usage of ocspgen:
        cfssl ocspgen -db-config db-config -ca cert -responder cert -responder-key key

Flags:
`

// Flags of 'cfssl ocspgen'
var ocspgenFlags = []string{"ca", "responder", "responder-key", "db-config"}

// ocspgenMain is the main CLI of OCSP generation functionality.
func ocspgenMain(args []string, c cli.Config) (err error) {
	s, err := SignerFromConfig(c)
	if err != nil {
		log.Critical("Unable to create OCSP signer: ", err)
		return err
	}

	if c.DBConfigFile == "" {
		log.Error("need DB config file (provide with -db-config)")
		return
	}

	var db *sql.DB
	db, err = certdb.DBFromConfig(c.DBConfigFile)
	if err != nil {
		return err
	}

	var certs []*certdb.CertificateRecord
	certs, err = certdb.GetUnexpiredCertificates(db)
	if err != nil {
		return err
	}

	for _, certRecord := range certs {
		cert, err := helpers.ParseCertificatePEM([]byte(certRecord.PEM))
		if err != nil {
			log.Critical("Unable to parse certificate: ", err)
			return err
		}

		req := ocsp.SignRequest{
			Certificate: cert,
			Status:      c.Status,
		}

		if certRecord.Status != "revoked" {
			req.Reason = int(certRecord.Reason)
			req.RevokedAt = certRecord.RevokedAt
		}

		resp, err := s.Sign(req)
		if err != nil {
			log.Critical("Unable to sign OCSP response: ", err)
			return err
		}

		// Update the OCSP record to expire in 1 day
		certdb.UpdateOCSP(nil, cert.SerialNumber.String(), string(resp), time.Now().AddDate(0, 0, 1))
	}

	var records []*certdb.OCSPRecord
	records, err = certdb.GetUnexpiredOCSPs(db)
	if err != nil {
		return err
	}
	for _, certRecord := range records {
		fmt.Printf("%s\n", base64.StdEncoding.EncodeToString([]byte(certRecord.Body)))
	}
	return nil
}

// SignerFromConfig creates a signer from a cli.Config as a helper for cli and serve
func SignerFromConfig(c cli.Config) (ocsp.Signer, error) {
	//if this is called from serve then we need to use the specific responder key file
	//fallback to key for backwards-compatibility
	k := c.ResponderKeyFile
	if k == "" {
		k = c.KeyFile
	}
	return ocsp.NewSignerFromFile(c.CAFile, c.ResponderFile, k, time.Duration(c.Interval))
}

// Command assembles the definition of Command 'ocspgen'
var Command = &cli.Command{UsageText: ocspgenUsageText, Flags: ocspgenFlags, Main: ocspgenMain}
