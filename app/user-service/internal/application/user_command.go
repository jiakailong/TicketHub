package application

import (
	"context"
	"strings"
	"time"

	"tickethub/app/user-service/internal/domain/user"
	"tickethub/pkg/auth"
	therrors "tickethub/pkg/errors"
)

type IDGenerator interface {
	NextID() (int64, error)
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash string, password string) bool
}

type RegisterCommand struct {
	Mobile   string
	Password string
}

type LoginCommand struct {
	Mobile   string
	Password string
}

type AddTicketUserCommand struct {
	UserID        int64
	Name          string
	CertificateNo string
	Mobile        string
}

type LoginResult struct {
	AccessToken string
	User        user.User
}

type UserCommandService struct {
	ids          IDGenerator
	users        user.Repository
	ticketUsers  user.TicketUserRepository
	hasher       PasswordHasher
	tokens       *auth.TokenManager
	nowFunc      func() time.Time
	adminMobiles map[string]struct{}
}

func NewUserCommandService(ids IDGenerator, users user.Repository, hasher PasswordHasher) UserCommandService {
	return UserCommandService{
		ids:     ids,
		users:   users,
		hasher:  hasher,
		nowFunc: time.Now,
	}
}

func (s UserCommandService) WithAdminMobiles(mobiles []string) UserCommandService {
	s.adminMobiles = make(map[string]struct{}, len(mobiles))
	for _, mobile := range mobiles {
		if value := strings.TrimSpace(mobile); value != "" {
			s.adminMobiles[value] = struct{}{}
		}
	}
	return s
}

func (s UserCommandService) WithTicketUsers(ticketUsers user.TicketUserRepository) UserCommandService {
	s.ticketUsers = ticketUsers
	return s
}

func (s UserCommandService) WithTokenManager(tokens auth.TokenManager) UserCommandService {
	s.tokens = &tokens
	return s
}

func (s UserCommandService) Register(ctx context.Context, cmd RegisterCommand) (user.User, error) {
	mobile := strings.TrimSpace(cmd.Mobile)
	if mobile == "" || strings.TrimSpace(cmd.Password) == "" {
		return user.User{}, therrors.New(therrors.CodeInvalidArgument, "mobile and password are required")
	}
	if _, err := s.users.FindByMobile(ctx, mobile); err == nil {
		return user.User{}, therrors.New(therrors.CodeConflict, "mobile already registered")
	} else if !therrors.IsCode(err, therrors.CodeNotFound) {
		return user.User{}, err
	}
	id, err := s.ids.NextID()
	if err != nil {
		return user.User{}, err
	}
	hash, err := s.hasher.Hash(cmd.Password)
	if err != nil {
		return user.User{}, err
	}
	registered := user.NewUser(id, mobile, hash, s.nowFunc())
	if err := s.users.Save(ctx, registered); err != nil {
		return user.User{}, err
	}
	return registered, nil
}

func (s UserCommandService) Login(ctx context.Context, cmd LoginCommand) (LoginResult, error) {
	mobile := strings.TrimSpace(cmd.Mobile)
	if mobile == "" || strings.TrimSpace(cmd.Password) == "" {
		return LoginResult{}, therrors.New(therrors.CodeInvalidArgument, "mobile and password are required")
	}
	current, err := s.users.FindByMobile(ctx, mobile)
	if err != nil {
		return LoginResult{}, err
	}
	if !s.hasher.Compare(current.PasswordHash, cmd.Password) {
		return LoginResult{}, therrors.New(therrors.CodeUnauthenticated, "invalid mobile or password")
	}
	token := ""
	if s.tokens != nil {
		role := "user"
		if _, ok := s.adminMobiles[current.Mobile]; ok {
			role = "admin"
		}
		token, err = s.tokens.Generate(auth.Claims{
			UserID:    current.ID,
			Role:      role,
			ExpiresAt: s.nowFunc().Add(24 * time.Hour).Unix(),
		})
		if err != nil {
			return LoginResult{}, err
		}
	}
	return LoginResult{AccessToken: token, User: current}, nil
}

func (s UserCommandService) GetUser(ctx context.Context, userID int64) (user.User, error) {
	if userID <= 0 {
		return user.User{}, therrors.New(therrors.CodeInvalidArgument, "user_id is required")
	}
	return s.users.FindByID(ctx, userID)
}

func (s UserCommandService) ListTicketUsers(ctx context.Context, userID int64) ([]user.TicketUser, error) {
	if s.ticketUsers == nil {
		return nil, therrors.New(therrors.CodeInfrastructure, "ticket user repository is not configured")
	}
	if userID <= 0 {
		return nil, therrors.New(therrors.CodeInvalidArgument, "user_id is required")
	}
	return s.ticketUsers.ListByUserID(ctx, userID)
}

func (s UserCommandService) AddTicketUser(ctx context.Context, cmd AddTicketUserCommand) (user.TicketUser, error) {
	if s.ticketUsers == nil {
		return user.TicketUser{}, therrors.New(therrors.CodeInfrastructure, "ticket user repository is not configured")
	}
	if cmd.UserID <= 0 || strings.TrimSpace(cmd.Name) == "" || strings.TrimSpace(cmd.CertificateNo) == "" {
		return user.TicketUser{}, therrors.New(therrors.CodeInvalidArgument, "user_id, name and certificate_no are required")
	}
	id, err := s.ids.NextID()
	if err != nil {
		return user.TicketUser{}, err
	}
	item := user.TicketUser{
		ID:            id,
		UserID:        cmd.UserID,
		Name:          strings.TrimSpace(cmd.Name),
		CertificateNo: strings.TrimSpace(cmd.CertificateNo),
		Mobile:        strings.TrimSpace(cmd.Mobile),
	}
	if err := s.ticketUsers.Save(ctx, item); err != nil {
		return user.TicketUser{}, err
	}
	return item, nil
}
