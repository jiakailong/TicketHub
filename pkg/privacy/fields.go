package privacy

import (
	"strings"
	"unicode"
)

func NormalizeMobile(value string) string {
	return strings.TrimSpace(value)
}

func NormalizeCertificate(value string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value))
}

func NormalizeName(value string) string {
	return strings.TrimSpace(value)
}

func NormalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func MaskMobile(value string) string {
	value = NormalizeMobile(value)
	runes := []rune(value)
	if len(runes) == 11 {
		return string(runes[:3]) + "****" + string(runes[7:])
	}
	return maskMiddle(runes, 2, 2)
}

func MaskCertificate(value string) string {
	value = NormalizeCertificate(value)
	return maskMiddle([]rune(value), 4, 4)
}

func MaskName(value string) string {
	runes := []rune(NormalizeName(value))
	if len(runes) == 0 {
		return ""
	}
	if len(runes) == 1 {
		return "*"
	}
	return string(runes[:1]) + strings.Repeat("*", len(runes)-1)
}

func MaskEmail(value string) string {
	value = NormalizeEmail(value)
	parts := strings.SplitN(value, "@", 2)
	if len(parts) != 2 {
		return maskMiddle([]rune(value), 2, 1)
	}
	local := []rune(parts[0])
	if len(local) <= 2 {
		return strings.Repeat("*", len(local)) + "@" + parts[1]
	}
	return string(local[:2]) + strings.Repeat("*", len(local)-2) + "@" + parts[1]
}

func maskMiddle(value []rune, prefix int, suffix int) string {
	if len(value) == 0 {
		return ""
	}
	if len(value) <= prefix+suffix {
		return strings.Repeat("*", len(value))
	}
	return string(value[:prefix]) + strings.Repeat("*", len(value)-prefix-suffix) + string(value[len(value)-suffix:])
}
