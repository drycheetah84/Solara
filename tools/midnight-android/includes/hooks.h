#pragma once
#include <cstddef>
#include "includes/FString.h"
#include "includes/opts.h"
#include "includes/url.h"

namespace Hooks {
    namespace InternalHooks {
        bool InternalProcessRequest(void* Req, bool isEOS);
    }

    namespace CurlHook {
        // [A2] curl_easy_setopt is NOT exported (dlsym fails), so we hook by offset.
        // 50451 (Android arm64): EOS is merged into libUnreal.so, so BOTH copies are in libUnreal.so.
        // Each copy has its own trampoline, so we keep a separate OG per copy.
        extern int (*OG_SETOPT_UE)(void*, int, void*);   // libUnreal.so + 0xa2b84bc (UE curl)   [was libUnreal.so+0x96aa490]
        extern int (*OG_SETOPT_EOS)(void*, int, void*);  // libUnreal.so + 0xa324f04 (EOS curl)  [was libEOSSDK.so+0x13fa898]
        void* SetupHook(void*);
        int SetOptHookUE(void* handle, int option, void* args);
        int SetOptHookEOS(void* handle, int option, void* args);
    }

    struct FCurlHook {
        static void (*SetUrl)(void*, const void*);
        static bool (*OGProcessRequest)(void*);

        FString::FString& GetUrl() { return *(FString::FString*)((uintptr_t)this + FCurlHookOpts::GETURL_ADDR); }
        static bool ProcessRequest(FCurlHook* Req) { return InternalHooks::InternalProcessRequest(Req, false); };
        static void* SetupHook(void* handle);
    };

    struct EOSFCurlHook {
        static void (*SetUrl)(void*, const void*);
        static bool (*OGProcessRequest)(void*);

        FString::FString& GetUrl() { return *(FString::FString*)((uintptr_t)this + EOSFCurlHookOpts::GETURL_ADDR); }
        static bool ProcessRequest(EOSFCurlHook* Req) { return InternalHooks::InternalProcessRequest(Req, true); };
        static void* SetupHook(void* handle);
    };
}
