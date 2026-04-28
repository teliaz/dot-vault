//go:build darwin && cgo

package biometric

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc -fblocks
#cgo darwin LDFLAGS: -framework Foundation -framework LocalAuthentication
#include <stdlib.h>
#include <string.h>
#include <dispatch/dispatch.h>
#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>

static char* dotVaultDupNSString(NSString *value) {
	if (value == nil) {
		return strdup("");
	}
	const char *utf8 = [value UTF8String];
	if (utf8 == NULL) {
		return strdup("");
	}
	return strdup(utf8);
}

static int dotVaultEvaluateTouchID(const char *reasonCString, char **errorCString) {
	@autoreleasepool {
		LAContext *context = [[LAContext alloc] init];
		NSError *availabilityError = nil;

		if (![context canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:&availabilityError]) {
			if (errorCString != NULL) {
				*errorCString = dotVaultDupNSString([availabilityError localizedDescription]);
			}
			return 0;
		}

		NSString *reason = [NSString stringWithUTF8String:reasonCString];
		dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);
		__block BOOL success = NO;
		__block NSString *failureMessage = nil;

		[context evaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics
				localizedReason:reason
						  reply:^(BOOL ok, NSError *evaluationError) {
			success = ok;
			if (evaluationError != nil) {
				failureMessage = [evaluationError localizedDescription];
			}
			dispatch_semaphore_signal(semaphore);
		}];

		dispatch_semaphore_wait(semaphore, DISPATCH_TIME_FOREVER);

		if (!success) {
			if (errorCString != NULL) {
				*errorCString = dotVaultDupNSString(failureMessage);
			}
			return 2;
		}

		return 1;
	}
}
*/
import "C"

import (
	"context"
	"fmt"
	"strings"
	"unsafe"

	"github.com/teliaz/dot-vault/internal/config"
)

type touchIDAuthorizer struct{}

func newPlatformAuthorizer() sensitiveAuthorizer {
	return touchIDAuthorizer{}
}

func (touchIDAuthorizer) Authorize(ctx context.Context, org config.Organization, action string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	reason := C.CString(fmt.Sprintf("Authorize %s for %s in dot-vault", action, org.Name))
	defer C.free(unsafe.Pointer(reason))

	var errorCString *C.char
	result := C.dotVaultEvaluateTouchID(reason, &errorCString)
	errorMessage := cStringAndFree(errorCString)

	switch result {
	case 1:
		return nil
	case 0:
		return fmt.Errorf("%w: %s", ErrTouchIDUnavailable, emptyAsDefault(errorMessage, "biometric authentication is unavailable"))
	default:
		return fmt.Errorf("touch id authorization failed: %s", emptyAsDefault(errorMessage, "authentication was not approved"))
	}
}

func cStringAndFree(value *C.char) string {
	if value == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value)
}

func emptyAsDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
