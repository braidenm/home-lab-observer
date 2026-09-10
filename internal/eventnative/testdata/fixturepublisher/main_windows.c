// This test-only publisher is compiled on the ephemeral runner against the
// Message Compiler header generated from the owned fixture manifest. It is
// never linked into an observer artifact.
#include <windows.h>
#include <evntprov.h>
#include <wchar.h>

#include "fixture.h"

static const GUID expected_provider = {
    0x4f0a8ead, 0x523c, 0x4f1d, {0x9d, 0x5e, 0x30, 0xd0, 0x3a, 0x30, 0xf8, 0x1b}
};

static BOOL descriptor_matches(const EVENT_DESCRIPTOR *descriptor, USHORT id,
                               UCHAR channel, UCHAR level) {
    return descriptor->Id == id && descriptor->Version == 0 &&
           descriptor->Channel == channel && descriptor->Level == level &&
           descriptor->Opcode == 0 && descriptor->Task == 0;
}

static BOOL generated_manifest_matches(void) {
    return IsEqualGUID(&HLO_FIXTURE_PROVIDER, &expected_provider) &&
           descriptor_matches(&HLO_SYSTEM_INFO, 101, 16, 4) &&
           descriptor_matches(&HLO_SYSTEM_WARN, 102, 16, 3) &&
           descriptor_matches(&HLO_SYSTEM_AFTER, 103, 16, 2) &&
           descriptor_matches(&HLO_APPLICATION_ERROR, 201, 17, 2) &&
           descriptor_matches(&HLO_APPLICATION_CRITICAL, 202, 17, 1) &&
           descriptor_matches(&HLO_APPLICATION_AFTER, 203, 17, 4);
}

static DWORD write_descriptors(REGHANDLE registration,
                               const EVENT_DESCRIPTOR *const *descriptors,
                               size_t count) {
    size_t index;
    ULONGLONG started = GetTickCount64();
    for (index = 0; index < count; ++index) {
        DWORD status;
        while (!EventEnabled(registration, descriptors[index])) {
            if (GetTickCount64() - started >= 10000) {
                return ERROR_NOT_READY;
            }
            Sleep(25);
        }
        status = EventWrite(registration, descriptors[index], 0, NULL);
        if (status != ERROR_SUCCESS) {
            return status;
        }
    }
    return ERROR_SUCCESS;
}

int wmain(int argc, wchar_t **argv) {
    const EVENT_DESCRIPTOR *before[] = {
        &HLO_SYSTEM_INFO,
        &HLO_SYSTEM_WARN,
        &HLO_APPLICATION_ERROR,
        &HLO_APPLICATION_CRITICAL,
    };
    const EVENT_DESCRIPTOR *after[] = {
        &HLO_SYSTEM_AFTER,
        &HLO_APPLICATION_AFTER,
    };
    const EVENT_DESCRIPTOR *const *selected;
    size_t count;
    REGHANDLE registration = 0;
    DWORD status;

    if (argc != 2) {
        return 2;
    }
    if (!generated_manifest_matches()) {
        return 4;
    }
    if (wcscmp(argv[1], L"before") == 0) {
        selected = before;
        count = ARRAYSIZE(before);
    } else if (wcscmp(argv[1], L"after") == 0) {
        selected = after;
        count = ARRAYSIZE(after);
    } else {
        return 2;
    }

    status = EventRegister(&HLO_FIXTURE_PROVIDER, NULL, NULL, &registration);
    if (status != ERROR_SUCCESS || registration == 0) {
        return 1;
    }
    status = write_descriptors(registration, selected, count);
    if (EventUnregister(registration) != ERROR_SUCCESS) {
        status = ERROR_INVALID_STATE;
    }
    if (status == ERROR_NOT_READY) {
        return 3;
    }
    return status == ERROR_SUCCESS ? 0 : 1;
}
