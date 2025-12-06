import {
  createContext,
  useContext,
  useState,
  useEffect,
  ReactNode,
} from "react";

interface User {
  id: string;
  username: string;
  email: string;
  role: string;
  used_storage: number;
  max_storage: number;
  created_at: string;
  updated_at: string;
  last_login?: string;
  status: string;
}

interface AuthContextType {
  user: User | null;
  token: string | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  register: (username: string, email: string, password: string) => Promise<void>;
  logout: () => void;
  isAuthenticated: boolean;
  clientCertificate: string | null;
  setupMTLS: (encryptionKey: string) => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const API_URL = "http://localhost:8081";

// Create a Certificate Signing Request using Web Crypto API
async function createCSR(privateKey: CryptoKey, publicKey: CryptoKey, username: string): Promise<string> {
  // Export public key in SPKI format
  const publicKeyData = await crypto.subtle.exportKey("spki", publicKey);
  const publicKeyB64 = btoa(String.fromCharCode(...new Uint8Array(publicKeyData)));

  // Create a simple CSR-like structure (not a real PKCS#10 CSR, but contains the public key)
  const csrData = {
    version: 1,
    subject: {
      commonName: `browser-${username}`,
      organizationName: "StorageOS",
    },
    publicKeyAlgorithm: "ecdsa-with-sha256",
    publicKey: publicKeyB64,
    signatureAlgorithm: "ecdsa-with-sha256",
  };

  // Convert to JSON and base64 encode
  const csrJson = JSON.stringify(csrData);
  const csrB64 = btoa(csrJson);

  return `-----BEGIN CERTIFICATE REQUEST-----
${csrB64.match(/.{1,64}/g)?.join('\n') || ''}
-----END CERTIFICATE REQUEST-----`;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [clientCertificate, setClientCertificate] = useState<string | null>(null);

  useEffect(() => {
    // Check for stored token on mount
    const storedToken = localStorage.getItem("auth_token");
    const storedUser = localStorage.getItem("auth_user");
    const storedCert = localStorage.getItem("client_certificate");

    if (storedToken && storedUser) {
      setToken(storedToken);
      setUser(JSON.parse(storedUser));
      // Optionally verify token with backend
      verifyToken(storedToken);
    } else {
      setLoading(false);
    }

    if (storedCert) {
      setClientCertificate(storedCert);
    }
  }, []);

  const verifyToken = async (authToken: string) => {
    try {
      const response = await fetch(`${API_URL}/v1/auth/me`, {
        headers: {
          Authorization: `Bearer ${authToken}`,
        },
      });

      if (response.ok) {
        const userData = await response.json();
        setUser(userData);
      } else {
        // Token is invalid, clear it
        localStorage.removeItem("auth_token");
        localStorage.removeItem("auth_user");
        setToken(null);
        setUser(null);
      }
    } catch (error) {
      console.error("Failed to verify token:", error);
      // Clear invalid token
      localStorage.removeItem("auth_token");
      localStorage.removeItem("auth_user");
      setToken(null);
      setUser(null);
    } finally {
      setLoading(false);
    }
  };

  const login = async (username: string, password: string) => {
    const response = await fetch(`${API_URL}/v1/auth/login`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ username, password }),
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || "Login failed");
    }

    const data = await response.json();
    setToken(data.token);
    setUser(data.user);
    localStorage.setItem("auth_token", data.token);
    localStorage.setItem("auth_user", JSON.stringify(data.user));
  };

  const register = async (
    username: string,
    email: string,
    password: string
  ) => {
    const response = await fetch(`${API_URL}/v1/auth/register`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ username, email, password }),
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || "Registration failed");
    }

    const data = await response.json();
    setToken(data.token);
    setUser(data.user);
    localStorage.setItem("auth_token", data.token);
    localStorage.setItem("auth_user", JSON.stringify(data.user));
  };

  const logout = () => {
    setToken(null);
    setUser(null);
    // Note: We keep the clientCertificate so users don't need to re-setup mTLS
    localStorage.removeItem("auth_token");
    localStorage.removeItem("auth_user");
    // Keep client_certificate in localStorage
    
    // Optionally call logout endpoint
    if (token) {
      fetch(`${API_URL}/v1/auth/logout`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
        },
      }).catch(console.error);
    }
  };

  const setupMTLS = async (encryptionKey: string) => {
    // Generate a temporary user ID for certificate generation
    const tempUserId = `temp-${Date.now()}`;

    // Generate ECDSA key pair
    const keyPair = await crypto.subtle.generateKey(
      {
        name: "ECDSA",
        namedCurve: "P-256",
      },
      true,
      ["sign", "verify"]
    );

    // Create CSR
    const csr = await createCSR(keyPair.privateKey, keyPair.publicKey, "browser-user");

    // Send to server
    const response = await fetch(`${API_URL}/v1/browser/get-certificate`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        encryption_key: encryptionKey,
        csr: csr,
        user_id: tempUserId,
      }),
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || "Failed to get certificate");
    }

    const data = await response.json();
    const certificate = data.certificate;

    // Store certificate
    setClientCertificate(certificate);
    localStorage.setItem("client_certificate", certificate);

    // Store private key (in production, this should be more secure)
    const privateKey = await crypto.subtle.exportKey("pkcs8", keyPair.privateKey);
    const privateKeyB64 = btoa(String.fromCharCode(...new Uint8Array(privateKey)));
    localStorage.setItem("client_private_key", privateKeyB64);
  };

  const value = {
    user,
    token,
    loading,
    login,
    register,
    logout,
    isAuthenticated: !!user && !!token,
    clientCertificate,
    setupMTLS,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
