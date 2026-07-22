package errors

func HTTPStatus(code Code) int {
	switch code {
	case CodeOK:
		return 200
	case CodeInvalidArgument:
		return 400
	case CodeUnauthenticated:
		return 401
	case CodeForbidden:
		return 403
	case CodeNotFound:
		return 404
	case CodeConflict, CodeSeatUnavailable, CodeOrderStateConflict, CodeDuplicateSubmission:
		return 409
	case CodeInventoryNotEnough:
		return 422
	case CodeCaptchaRequired:
		return 428
	case CodeCaptchaInvalid:
		return 400
	case CodeTooManyRequests:
		return 429
	case CodeInfrastructure:
		return 503
	default:
		return 500
	}
}

func PublicMessage(code Code) string {
	switch code {
	case CodeInventoryNotEnough:
		return "库存不足"
	case CodeSeatUnavailable:
		return "座位不可售"
	case CodeOrderStateConflict:
		return "订单状态冲突"
	case CodeDuplicateSubmission:
		return "重复提交"
	case CodeCaptchaRequired:
		return "请完成验证码验证"
	case CodeCaptchaInvalid:
		return "验证码无效或已过期"
	default:
		return string(code)
	}
}
