package auth

import (
	"crypto/rand"
	"errors"
	"math/big"
	"regexp"

	"golang.org/x/crypto/bcrypt"
)

// Role identifies which subdomain / cabinet a credential belongs to.
type Role string

const (
	RoleStudent    Role = "student"
	RoleParent     Role = "parent"
	RoleTeacher    Role = "teacher"
	RoleDirector   Role = "director"
	RoleSuperAdmin Role = "super_admin"
)

var (
	ErrPasswordPolicy = errors.New("password does not satisfy the policy for this role")

	pinRegex          = regexp.MustCompile(`^\d{4}$`)
	teacherRegex      = regexp.MustCompile(`^[A-Za-z0-9]{6,}$`)
	teacherHasLetter  = regexp.MustCompile(`[A-Za-z]`)
	teacherHasDigit   = regexp.MustCompile(`\d`)
	complexPassRegex  = regexp.MustCompile(`^[A-Za-z0-9!@#$%^&*()_+\-=]+$`)
	complexHasUpper   = regexp.MustCompile(`[A-Z]`)
	complexHasLower   = regexp.MustCompile(`[a-z]`)
	complexHasDigit   = regexp.MustCompile(`\d`)
	complexHasSpecial = regexp.MustCompile(`[!@#$%^&*()_+\-=]`)
)

// ValidatePasswordPolicy enforces the exact rules from the dosedu.kz spec:
//
//	Student / Parent : exactly 4 digits
//	Teacher          : min 6 chars, letters AND digits mixed
//	Director         : exactly 8 chars, complex (upper+lower+digit+special)
//	Super Admin      : exactly 10 chars, complex (upper+lower+digit+special)
func ValidatePasswordPolicy(role Role, plain string) error {
	switch role {
	case RoleStudent, RoleParent:
		if !pinRegex.MatchString(plain) {
			return ErrPasswordPolicy
		}
	case RoleTeacher:
		if !teacherRegex.MatchString(plain) ||
			!teacherHasLetter.MatchString(plain) ||
			!teacherHasDigit.MatchString(plain) {
			return ErrPasswordPolicy
		}
	case RoleDirector:
		if len(plain) != 8 || !isComplex(plain) {
			return ErrPasswordPolicy
		}
	case RoleSuperAdmin:
		if len(plain) != 10 || !isComplex(plain) {
			return ErrPasswordPolicy
		}
	default:
		return errors.New("unknown role")
	}
	return nil
}

func isComplex(plain string) bool {
	return complexPassRegex.MatchString(plain) &&
		complexHasUpper.MatchString(plain) &&
		complexHasLower.MatchString(plain) &&
		complexHasDigit.MatchString(plain) &&
		complexHasSpecial.MatchString(plain)
}

// GenerateComplexPassword produces a random password satisfying the
// complex-password policy (upper+lower+digit+special) at the given
// exact length, used when auto-provisioning director/super-admin
// accounts. The result always validates against ValidatePasswordPolicy
// for the given role.
func GenerateComplexPassword(role Role, length int) (string, error) {
	const (
		upper   = "ABCDEFGHJKLMNPQRSTUVWXYZ" // no O/I, avoids visual ambiguity
		lower   = "abcdefghijkmnpqrstuvwxyz" // no l/o
		digits  = "23456789"                 // no 0/1
		special = "!@#$%^&*()_+-="
	)
	all := upper + lower + digits + special

	if length < 4 {
		return "", errors.New("length too short for a complex password")
	}

	buf := make([]byte, length)
	// Guarantee at least one char from each required class first.
	classes := []string{upper, lower, digits, special}
	for i, class := range classes {
		ch, err := randomByte(class)
		if err != nil {
			return "", err
		}
		buf[i] = ch
	}
	for i := len(classes); i < length; i++ {
		ch, err := randomByte(all)
		if err != nil {
			return "", err
		}
		buf[i] = ch
	}

	// Shuffle so the guaranteed classes aren't always in the same
	// leading positions.
	for i := length - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		j := int(jBig.Int64())
		buf[i], buf[j] = buf[j], buf[i]
	}

	pw := string(buf)
	if err := ValidatePasswordPolicy(role, pw); err != nil {
		// Astronomically unlikely given the construction above, but
		// guard against it rather than ever returning a policy-invalid
		// password.
		return GenerateComplexPassword(role, length)
	}
	return pw, nil
}

// GenerateTeacherPassword produces a random password satisfying the
// teacher policy (6+ chars, letters+digits mixed, NO special
// characters) at the given length.
//
// This is deliberately separate from GenerateComplexPassword: that
// generator always includes a special character (by construction, one
// per required class: upper/lower/digit/special), which the teacher
// policy's regex (`^[A-Za-z0-9]{6,}$`) rejects outright — calling
// GenerateComplexPassword(RoleTeacher, ...) would make its
// validate-and-retry loop recurse forever, since no output it can ever
// produce satisfies the teacher policy.
func GenerateTeacherPassword(length int) (string, error) {
	const (
		letters = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz" // no O/I/l/o
		digits  = "23456789"                                         // no 0/1
	)
	all := letters + digits

	if length < 2 {
		return "", errors.New("length too short for a teacher password")
	}

	buf := make([]byte, length)
	letterCh, err := randomByte(letters)
	if err != nil {
		return "", err
	}
	digitCh, err := randomByte(digits)
	if err != nil {
		return "", err
	}
	buf[0], buf[1] = letterCh, digitCh
	for i := 2; i < length; i++ {
		ch, err := randomByte(all)
		if err != nil {
			return "", err
		}
		buf[i] = ch
	}

	for i := length - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		j := int(jBig.Int64())
		buf[i], buf[j] = buf[j], buf[i]
	}

	pw := string(buf)
	if err := ValidatePasswordPolicy(RoleTeacher, pw); err != nil {
		// Astronomically unlikely given the construction above, but
		// guard against it rather than ever returning a policy-invalid
		// password.
		return GenerateTeacherPassword(length)
	}
	return pw, nil
}

func randomByte(charset string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
	if err != nil {
		return 0, err
	}
	return charset[n.Int64()], nil
}

// HashPassword bcrypt-hashes a password after confirming it satisfies the
// role's policy. Always validate before hashing so bad credentials never
// reach the database.
func HashPassword(role Role, plain string) (string, error) {
	if err := ValidatePasswordPolicy(role, plain); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword compares a plaintext credential against a stored bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
