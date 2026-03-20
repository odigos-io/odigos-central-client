package odigos

const SignInMutation = `mutation SignIn($email: String, $password: String, $code: String) {
		signIn(email: $email, password: $password, code: $code) {
			idToken
			accessToken
			status
			user {
				id
				email
				username
				role
				needsPasswordChange
				teams {
					id
					name
					description
				}
				computePlatforms {
					id
					name
					type
					status
				}
			}
		}
	}`

const PersistSourcesMutation = `mutation PersistSources($sources: [PersistNamespaceSourceInput!]!) {
		persistK8sSources(sources: $sources)
	}`

const PersistNamespaceSourcesMutation = `mutation PersistNamespaces($namespaces: [PersistNamespaceItemInput!]!) {
		persistK8sNamespaces(namespaces: $namespaces)
	}`
