package minio

import "github.com/minio/madmin-go/v3"

// mapServiceAccountInfo converts a madmin service account entry to our native type.
func mapServiceAccountInfo(a madmin.ServiceAccountInfo) ServiceAccountInfo {
	return ServiceAccountInfo{
		AccessKey:     a.AccessKey,
		ParentUser:    a.ParentUser,
		AccountStatus: a.AccountStatus,
		Name:          a.Name,
		Expiration:    a.Expiration,
	}
}

// mapCredentials converts madmin credentials to our native type.
func mapCredentials(c madmin.Credentials) Credentials {
	return Credentials{
		AccessKey: c.AccessKey,
		SecretKey: c.SecretKey,
	}
}

// mapInfoServiceAccountResp converts the madmin service account info response.
func mapInfoServiceAccountResp(info madmin.InfoServiceAccountResp) ServiceAccountInfoResp {
	return ServiceAccountInfoResp{
		ParentUser:    info.ParentUser,
		AccountStatus: info.AccountStatus,
		Name:          info.Name,
		Description:   info.Description,
		ImpliedPolicy: info.ImpliedPolicy,
		Expiration:    info.Expiration,
	}
}

// mapUserInfo converts a madmin IAM user entry to our native type.
// The AccountStatus string type is cast — its values are identical.
func mapUserInfo(u madmin.UserInfo) UserInfo {
	return UserInfo{
		Status:     AccountStatus(u.Status),
		PolicyName: u.PolicyName,
		MemberOf:   u.MemberOf,
		UpdatedAt:  u.UpdatedAt,
	}
}

// mapPolicyAssociationResp converts the madmin attach/detach policy response.
func mapPolicyAssociationResp(r madmin.PolicyAssociationResp) PolicyAssociationResp {
	return PolicyAssociationResp{
		PoliciesDetached: r.PoliciesDetached,
		PoliciesAttached: r.PoliciesAttached,
	}
}

// mapPolicyInfo converts a madmin policy info entry to our native type.
func mapPolicyInfo(info madmin.PolicyInfo) PolicyInfo {
	return PolicyInfo{
		PolicyName: info.PolicyName,
		Policy:     info.Policy,
		CreateDate: info.CreateDate,
		UpdateDate: info.UpdateDate,
	}
}
