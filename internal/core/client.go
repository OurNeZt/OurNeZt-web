package core

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"github.com/OurNeZt/ournezt-web/internal/config"
	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Clients struct {
	conn      *grpc.ClientConn
	Auth      ourneztv1.AuthServiceClient
	Family    ourneztv1.FamilyServiceClient
	Person    ourneztv1.PersonServiceClient
	Housing   ourneztv1.HousingServiceClient
	Income    ourneztv1.IncomeServiceClient
	CPF       ourneztv1.CPFServiceClient
	Dashboard ourneztv1.DashboardServiceClient
}

func NewClients(ctx context.Context, cfg config.Config) (*Clients, error) {
	var transportCreds credentials.TransportCredentials
	if cfg.CoreGRPCUseTLS {
		tlsConfig := &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: cfg.CoreGRPCInsecure, //nolint:gosec // explicitly controlled via env for non-production interop only
		}
		if serverName := strings.TrimSpace(cfg.CoreGRPCTLSServer); serverName != "" {
			tlsConfig.ServerName = serverName
		}

		if caPath := strings.TrimSpace(cfg.CoreGRPCTLSCAFile); caPath != "" {
			caBytes, readErr := os.ReadFile(caPath)
			if readErr != nil {
				return nil, fmt.Errorf("read core grpc CA file: %w", readErr)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caBytes) {
				return nil, fmt.Errorf("parse core grpc CA file: invalid PEM")
			}
			tlsConfig.RootCAs = pool
		}

		transportCreds = credentials.NewTLS(tlsConfig)
	} else {
		transportCreds = insecure.NewCredentials()
	}

	conn, err := grpc.NewClient(cfg.CoreGRPCAddr, grpc.WithTransportCredentials(transportCreds))
	if err != nil {
		return nil, err
	}

	return &Clients{
		conn:      conn,
		Auth:      ourneztv1.NewAuthServiceClient(conn),
		Family:    ourneztv1.NewFamilyServiceClient(conn),
		Person:    ourneztv1.NewPersonServiceClient(conn),
		Housing:   ourneztv1.NewHousingServiceClient(conn),
		Income:    ourneztv1.NewIncomeServiceClient(conn),
		CPF:       ourneztv1.NewCPFServiceClient(conn),
		Dashboard: ourneztv1.NewDashboardServiceClient(conn),
	}, nil
}

func (c *Clients) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
