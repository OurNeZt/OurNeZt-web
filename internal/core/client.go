package core

import (
	"context"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"google.golang.org/grpc"
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

func NewClients(ctx context.Context, grpcAddr string) (*Clients, error) {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
