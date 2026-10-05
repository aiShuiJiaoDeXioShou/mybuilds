//go:build darwin && cgo

package mobile

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#cgo CFLAGS: -Wno-deprecated-declarations
#include <Security/Security.h>
#include <Security/CMSDecoder.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static CFDataRef iosData(const unsigned char *data,size_t length) {
 return CFDataCreate(kCFAllocatorDefault,data,(CFIndex)length);
}
static CFStringRef iosString(const char *value) {
 return CFStringCreateWithCString(kCFAllocatorDefault,value,kCFStringEncodingUTF8);
}
static int iosAvailable(void) {
 if (__builtin_available(macOS 15.0, *)) return 1;
 return 0;
}
static int iosP12(CFDataRef data,CFStringRef password,SecKeychainRef keychain,SecAccessRef access,CFDataRef *certificate) {
 if(!iosAvailable()) return 1;
 CFMutableDictionaryRef options=CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFDictionarySetValue(options,kSecImportExportPassphrase,password);
 if(keychain) {
  CFDictionarySetValue(options,kSecImportExportKeychain,keychain);
  CFDictionarySetValue(options,kSecImportExportAccess,access);
 } else {
  if (__builtin_available(macOS 15.0,*)) CFDictionarySetValue(options,kSecImportToMemoryOnly,kCFBooleanTrue);
 }
 CFArrayRef items=NULL;OSStatus status=SecPKCS12Import(data,options,&items);CFRelease(options);
 int result=1;
 if(status==errSecSuccess && items && CFArrayGetCount(items)==1) {
  CFDictionaryRef item=CFArrayGetValueAtIndex(items,0);
  SecIdentityRef identity=(SecIdentityRef)CFDictionaryGetValue(item,kSecImportItemIdentity);
  SecCertificateRef cert=NULL;SecKeyRef key=NULL;
  if(identity && SecIdentityCopyPrivateKey(identity,&key)==errSecSuccess && SecIdentityCopyCertificate(identity,&cert)==errSecSuccess) {
   *certificate=SecCertificateCopyData(cert);result=(*certificate==NULL);
  }
  if(cert)CFRelease(cert);if(key)CFRelease(key);
 }
 if(items)CFRelease(items);return result;
}
static CFArrayRef iosAnchors(const unsigned char *bundle,size_t length) {
 CFMutableArrayRef anchors=CFArrayCreateMutable(NULL,0,&kCFTypeArrayCallBacks);
 size_t offset=0;
 while(offset+4<=length) {
  size_t size=((size_t)bundle[offset]<<24)|((size_t)bundle[offset+1]<<16)|((size_t)bundle[offset+2]<<8)|bundle[offset+3];offset+=4;
  if(size==0||size>length-offset) {CFRelease(anchors);return NULL;}
  CFDataRef data=iosData(bundle+offset,size);SecCertificateRef cert=SecCertificateCreateWithData(NULL,data);CFRelease(data);
  if(!cert){CFRelease(anchors);return NULL;}CFArrayAppendValue(anchors,cert);CFRelease(cert);offset+=size;
 }
 if(offset!=length||CFArrayGetCount(anchors)!=3){CFRelease(anchors);return NULL;}
 return anchors;
}
static int iosInspect(const unsigned char *p12,size_t p12Length,const unsigned char *profile,size_t profileLength,const char *password,const unsigned char *roots,size_t rootsLength,CFDataRef *certificate,CFDataRef *plist,CFDataRef *signer) {
 int result=1;CFDataRef p12Data=iosData(p12,p12Length);CFStringRef pass=iosString(password);
 if(iosP12(p12Data,pass,NULL,NULL,certificate)!=0) {CFRelease(p12Data);CFRelease(pass);return 1;}
 CFRelease(p12Data);CFRelease(pass);result=2;
 CMSDecoderRef decoder=NULL;CFArrayRef anchors=NULL;SecPolicyRef policy=NULL;CFDataRef content=NULL;CFPropertyListRef property=NULL;
 if(CMSDecoderCreate(&decoder)!=errSecSuccess||CMSDecoderUpdateMessage(decoder,profile,profileLength)!=errSecSuccess||CMSDecoderFinalizeMessage(decoder)!=errSecSuccess) goto done;
 size_t count=0;if(CMSDecoderGetNumSigners(decoder,&count)!=errSecSuccess||count!=1)goto done;
 anchors=iosAnchors(roots,rootsLength);policy=SecPolicyCreateBasicX509();if(!anchors||!policy)goto done;
 CMSSignerStatus status=kCMSSignerUnsigned;SecTrustRef trust=NULL;
 if(CMSDecoderCopySignerStatus(decoder,0,policy,false,&status,&trust,NULL)!=errSecSuccess||status!=kCMSSignerValid||!trust) {if(trust)CFRelease(trust);goto done;}
 Boolean valid=SecTrustSetAnchorCertificates(trust,anchors)==errSecSuccess&&SecTrustSetAnchorCertificatesOnly(trust,true)==errSecSuccess&&SecTrustSetNetworkFetchAllowed(trust,false)==errSecSuccess&&SecTrustEvaluateWithError(trust,NULL);
 CFRelease(trust);if(!valid)goto done;
 SecCertificateRef signerCert=NULL;
 if(CMSDecoderCopySignerCert(decoder,0,&signerCert)!=errSecSuccess||!signerCert)goto done;
 *signer=SecCertificateCopyData(signerCert);CFRelease(signerCert);if(!*signer)goto done;
 if(CMSDecoderCopyContent(decoder,&content)!=errSecSuccess||!content)goto done;
 property=CFPropertyListCreateWithData(NULL,content,kCFPropertyListImmutable,NULL,NULL);if(!property)goto done;
 *plist=CFPropertyListCreateData(NULL,property,kCFPropertyListXMLFormat_v1_0,0,NULL);if(!*plist)goto done;
 result=0;
 done:
 if(property)CFRelease(property);if(content)CFRelease(content);if(policy)CFRelease(policy);if(anchors)CFRelease(anchors);if(decoder)CFRelease(decoder);return result;
}
static int iosImport(const unsigned char *p12,size_t length,const char *password,const char *keychainPath,const char *keychainPassword) {
 SecKeychainRef keychain=NULL;SecTrustedApplicationRef codesign=NULL;SecAccessRef access=NULL;CFArrayRef applications=NULL;CFStringRef label=NULL;CFDataRef data=NULL,certificate=NULL;CFStringRef pass=NULL;
 int result=1;
 if(SecTrustedApplicationCreateFromPath("/usr/bin/codesign",&codesign)!=errSecSuccess)goto done;
 const void *values[]={codesign};applications=CFArrayCreate(NULL,values,1,&kCFTypeArrayCallBacks);label=iosString("mybuilds iOS signing");
 if(SecAccessCreate(label,applications,&access)!=errSecSuccess)goto done;
 if(SecKeychainCreate(keychainPath,(UInt32)strlen(keychainPassword),keychainPassword,false,access,&keychain)!=errSecSuccess)goto done;
 if(SecKeychainUnlock(keychain,(UInt32)strlen(keychainPassword),keychainPassword,true)!=errSecSuccess)goto done;
 data=iosData(p12,length);pass=iosString(password);
 if(iosP12(data,pass,keychain,access,&certificate)!=0)goto done;
 result=0;
 done:
 if(pass)CFRelease(pass);if(data)CFRelease(data);if(certificate)CFRelease(certificate);if(access)CFRelease(access);if(applications)CFRelease(applications);if(label)CFRelease(label);if(codesign)CFRelease(codesign);if(keychain)CFRelease(keychain);return result;
}
static int iosDelete(const char *path) {
 SecKeychainRef keychain=NULL;OSStatus status=SecKeychainOpen(path,&keychain);
 if(status==errSecNoSuchKeychain) return 0;
 if(status!=errSecSuccess||!keychain)return 1;
 status=SecKeychainDelete(keychain);CFRelease(keychain);return status!=errSecSuccess;
}
*/
import "C"

import (
	"errors"
	"os"
	"unsafe"
)

func iosNativeAvailable() bool { return C.iosAvailable() != 0 }
func iosNativeInspect(p12, profile []byte, password string) (certificate, plist, signer []byte, err error) {
	if !iosNativeAvailable() {
		return nil, nil, nil, errors.New("ios_signing_unsupported")
	}
	roots := iosRootBundle()
	if len(p12) == 0 || len(profile) == 0 || len(roots) == 0 {
		return nil, nil, nil, errors.New("ios_material_invalid")
	}
	pass := C.CString(password)
	defer C.free(unsafe.Pointer(pass))
	var certData, plistData, signerData C.CFDataRef
	result := C.iosInspect((*C.uchar)(unsafe.Pointer(&p12[0])), C.size_t(len(p12)), (*C.uchar)(unsafe.Pointer(&profile[0])), C.size_t(len(profile)), pass, (*C.uchar)(unsafe.Pointer(&roots[0])), C.size_t(len(roots)), &certData, &plistData, &signerData)
	for _, data := range []C.CFDataRef{certData, plistData, signerData} {
		if data != 0 {
			defer C.CFRelease(C.CFTypeRef(data))
		}
	}
	if result != 0 {
		if result == 1 {
			return nil, nil, nil, errors.New("ios_p12_invalid")
		}
		return nil, nil, nil, errors.New("ios_profile_untrusted")
	}
	return iosCopyData(certData), iosCopyData(plistData), iosCopyData(signerData), nil
}
func iosCopyData(data C.CFDataRef) []byte {
	return C.GoBytes(unsafe.Pointer(C.CFDataGetBytePtr(data)), C.int(C.CFDataGetLength(data)))
}
func iosNativeImport(p12 []byte, password, keychain, keychainPassword string) error {
	if !iosNativeAvailable() {
		return errors.New("ios_signing_unsupported")
	}
	if len(p12) == 0 {
		return errors.New("ios_p12_invalid")
	}
	pass := C.CString(password)
	keychainPath := C.CString(keychain)
	keychainPass := C.CString(keychainPassword)
	defer C.free(unsafe.Pointer(pass))
	defer C.free(unsafe.Pointer(keychainPath))
	defer C.free(unsafe.Pointer(keychainPass))
	if C.iosImport((*C.uchar)(unsafe.Pointer(&p12[0])), C.size_t(len(p12)), pass, keychainPath, keychainPass) != 0 {
		return errors.New("ios_keychain_failed")
	}
	return nil
}
func iosNativeDelete(keychain string) error {
	exists := false
	for _, filename := range []string{keychain, keychain + "-db"} {
		info, err := os.Lstat(filename)
		if err != nil && !os.IsNotExist(err) {
			return errors.New("ios_cleanup_failed")
		}
		if err == nil {
			exists = true
		}
		if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
			return errors.New("ios_cleanup_failed")
		}
	}
	if !exists {
		return nil
	}
	keychainPath := C.CString(keychain)
	defer C.free(unsafe.Pointer(keychainPath))
	if C.iosDelete(keychainPath) != 0 {
		return errors.New("ios_cleanup_failed")
	}
	for _, filename := range []string{keychain, keychain + "-db"} {
		if _, err := os.Lstat(filename); !os.IsNotExist(err) {
			return errors.New("ios_cleanup_failed")
		}
	}
	return nil
}
