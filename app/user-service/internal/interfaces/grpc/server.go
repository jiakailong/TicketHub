package grpcapi

import (
	"context"

	userv1 "tickethub/api/proto/user/v1"
	"tickethub/app/user-service/internal/application"
	"tickethub/pkg/privacy"
)

type Server struct {
	userv1.UnimplementedUserServiceServer
	users application.UserCommandService
}

func NewServer(users application.UserCommandService) Server {
	return Server{users: users}
}

func (s Server) Register(ctx context.Context, req *userv1.RegisterRequest) (*userv1.UserReply, error) {
	created, err := s.users.Register(ctx, application.RegisterCommand{
		Mobile:   req.GetMobile(),
		Password: req.GetPassword(),
	})
	if err != nil {
		return nil, err
	}
	return userReply(created.ID, created.Mobile, string(created.RealNameStatus)), nil
}

func (s Server) Login(ctx context.Context, req *userv1.LoginRequest) (*userv1.LoginReply, error) {
	result, err := s.users.Login(ctx, application.LoginCommand{
		Mobile:   req.GetMobile(),
		Password: req.GetPassword(),
	})
	if err != nil {
		return nil, err
	}
	return &userv1.LoginReply{
		AccessToken: result.AccessToken,
		User:        userReply(result.User.ID, result.User.Mobile, string(result.User.RealNameStatus)),
	}, nil
}

func (s Server) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.UserReply, error) {
	current, err := s.users.GetUser(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	return userReply(current.ID, current.Mobile, string(current.RealNameStatus)), nil
}

func (s Server) ListTicketUsers(ctx context.Context, req *userv1.ListTicketUsersRequest) (*userv1.ListTicketUsersReply, error) {
	items, err := s.users.ListTicketUsers(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	reply := &userv1.ListTicketUsersReply{TicketUsers: make([]*userv1.TicketUser, 0, len(items))}
	for _, item := range items {
		reply.TicketUsers = append(reply.TicketUsers, &userv1.TicketUser{
			Id:            item.ID,
			Name:          privacy.MaskName(item.Name),
			CertificateNo: privacy.MaskCertificate(item.CertificateNo),
		})
	}
	return reply, nil
}

func (s Server) AddTicketUser(ctx context.Context, req *userv1.AddTicketUserRequest) (*userv1.TicketUser, error) {
	item, err := s.users.AddTicketUser(ctx, application.AddTicketUserCommand{
		UserID:        req.GetUserId(),
		Name:          req.GetName(),
		CertificateNo: req.GetCertificateNo(),
		Mobile:        req.GetMobile(),
	})
	if err != nil {
		return nil, err
	}
	return &userv1.TicketUser{Id: item.ID, Name: privacy.MaskName(item.Name), CertificateNo: privacy.MaskCertificate(item.CertificateNo)}, nil
}

func userReply(id int64, mobile string, realNameStatus string) *userv1.UserReply {
	return &userv1.UserReply{
		UserId:         id,
		Mobile:         privacy.MaskMobile(mobile),
		RealNameStatus: realNameStatus,
	}
}
