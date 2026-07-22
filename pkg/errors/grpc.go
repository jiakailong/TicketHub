package errors

import (
	"context"

	"github.com/go-kratos/kratos/v2/middleware"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func GRPCServerMiddleware() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			response, err := next(ctx, req)
			if err != nil {
				return nil, ToGRPC(err)
			}
			return response, nil
		}
	}
}

func ToGRPC(err error) error {
	if err == nil {
		return nil
	}
	code := CodeOf(err)
	if code == CodeInternal {
		if _, ok := status.FromError(err); ok {
			return err
		}
	}
	value := status.New(grpcCode(code), PublicMessage(code))
	withDetails, detailErr := value.WithDetails(&errdetails.ErrorInfo{
		Reason: string(code),
		Domain: "tickethub",
	})
	if detailErr == nil {
		value = withDetails
	}
	return value.Err()
}

func codeFromGRPC(err error) (Code, bool) {
	value, ok := status.FromError(err)
	if !ok {
		return CodeInternal, false
	}
	for _, detail := range value.Details() {
		info, ok := detail.(*errdetails.ErrorInfo)
		if !ok || info.GetDomain() != "tickethub" {
			continue
		}
		code := Code(info.GetReason())
		if isKnownCode(code) {
			return code, true
		}
	}
	return codeFromGRPCStatus(value.Code()), true
}

func grpcCode(code Code) codes.Code {
	switch code {
	case CodeInvalidArgument:
		return codes.InvalidArgument
	case CodeUnauthenticated:
		return codes.Unauthenticated
	case CodeForbidden:
		return codes.PermissionDenied
	case CodeNotFound:
		return codes.NotFound
	case CodeConflict, CodeSeatUnavailable, CodeOrderStateConflict, CodeDuplicateSubmission, CodeCaptchaRequired:
		return codes.FailedPrecondition
	case CodeCaptchaInvalid:
		return codes.InvalidArgument
	case CodeInventoryNotEnough:
		return codes.ResourceExhausted
	case CodeTooManyRequests:
		return codes.ResourceExhausted
	case CodeInfrastructure:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}

func codeFromGRPCStatus(code codes.Code) Code {
	switch code {
	case codes.InvalidArgument:
		return CodeInvalidArgument
	case codes.Unauthenticated:
		return CodeUnauthenticated
	case codes.PermissionDenied:
		return CodeForbidden
	case codes.NotFound:
		return CodeNotFound
	case codes.FailedPrecondition, codes.AlreadyExists, codes.Aborted:
		return CodeConflict
	case codes.ResourceExhausted:
		return CodeTooManyRequests
	case codes.Unavailable, codes.DeadlineExceeded:
		return CodeInfrastructure
	default:
		return CodeInternal
	}
}

func isKnownCode(code Code) bool {
	switch code {
	case CodeOK, CodeInvalidArgument, CodeUnauthenticated, CodeForbidden, CodeNotFound,
		CodeConflict, CodeTooManyRequests, CodeInventoryNotEnough, CodeSeatUnavailable,
		CodeOrderStateConflict, CodeDuplicateSubmission, CodeCaptchaRequired, CodeCaptchaInvalid,
		CodeInfrastructure, CodeInternal:
		return true
	default:
		return false
	}
}
