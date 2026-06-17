//go:build windows

package weixin

import (
	"os"

	"golang.org/x/sys/windows"
)

// writeSecureFile 用 Windows ACL 写入只对当前用户可见的敏感状态。
func writeSecureFile(path string, content []byte) error {
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return err
	}
	return secureStorePath(path, false)
}

// secureStorePath 固定敏感状态目录或文件的私有访问权限。
func secureStorePath(path string, directory bool) error {
	currentToken := windows.GetCurrentProcessToken()
	currentUser, err := currentToken.GetTokenUser()
	if err != nil {
		return err
	}
	adminSid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	systemSid, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}

	inheritance := uint32(0)
	if directory {
		inheritance = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	entries := []windows.EXPLICIT_ACCESS{
		secureAccessEntry(currentUser.User.Sid, windows.TRUSTEE_IS_USER, inheritance),
		secureAccessEntry(adminSid, windows.TRUSTEE_IS_GROUP, inheritance),
		secureAccessEntry(systemSid, windows.TRUSTEE_IS_USER, inheritance),
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
}

// secureAccessEntry 构造授予指定 SID 完全控制权的 ACL 项。
func secureAccessEntry(sid *windows.SID, trusteeType windows.TRUSTEE_TYPE, inheritance uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trusteeType,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}
