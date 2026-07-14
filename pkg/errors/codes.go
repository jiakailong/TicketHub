package errors

type Code string

const (
	CodeOK                  Code = "OK"
	CodeInvalidArgument     Code = "INVALID_ARGUMENT"
	CodeUnauthenticated     Code = "UNAUTHENTICATED"
	CodeForbidden           Code = "FORBIDDEN"
	CodeNotFound            Code = "NOT_FOUND"
	CodeConflict            Code = "CONFLICT"
	CodeTooManyRequests     Code = "TOO_MANY_REQUESTS"
	CodeInventoryNotEnough  Code = "INVENTORY_NOT_ENOUGH"
	CodeSeatUnavailable     Code = "SEAT_UNAVAILABLE"
	CodeOrderStateConflict  Code = "ORDER_STATE_CONFLICT"
	CodeDuplicateSubmission Code = "DUPLICATE_SUBMISSION"
	CodeInfrastructure      Code = "INFRASTRUCTURE_ERROR"
	CodeInternal            Code = "INTERNAL"
)
