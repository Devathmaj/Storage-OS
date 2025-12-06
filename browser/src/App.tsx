import { Toaster } from "@/components/ui/toaster";
import { Toaster as Sonner } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { AuthProvider, useAuth } from "./contexts/AuthContext";
import Index from "./pages/Index";
import RecentPage from "./pages/RecentPage";
import Favorites from "./pages/Favorites";
import Trash from "./pages/Trash";
import SettingsPage from "./pages/SettingsPage";
import UserProfile from "./pages/UserProfile";
import UserManagement from "./pages/UserManagement";
import LoginPage from "./pages/LoginPage";
import NotFound from "./pages/NotFound";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";

const queryClient = new QueryClient();

// MTLS Setup Component
function MTLSSetup() {
  const { setupMTLS } = useAuth();
  const { toast } = useToast();
  const [encryptionKey, setEncryptionKey] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSetup = async () => {
    if (!encryptionKey.trim()) {
      toast({
        title: "Error",
        description: "Please enter the encryption key",
        variant: "destructive",
      });
      return;
    }

    setLoading(true);
    try {
      await setupMTLS(encryptionKey);
      toast({
        title: "Success",
        description: "mTLS certificate configured successfully",
      });
    } catch (error) {
      toast({
        title: "Error",
        description: error instanceof Error ? error.message : "Failed to setup mTLS",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex items-center justify-center min-h-screen bg-gray-50">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Setup Secure Connection</CardTitle>
          <CardDescription>
            Enter the encryption key from your controller to establish a secure connection.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <label htmlFor="key" className="block text-sm font-medium text-gray-700 mb-1">
              Encryption Key
            </label>
            <Input
              id="key"
              type="password"
              value={encryptionKey}
              onChange={(e) => setEncryptionKey(e.target.value)}
              placeholder="Enter encryption key"
              className="w-full"
            />
          </div>
          <Button
            onClick={handleSetup}
            disabled={loading}
            className="w-full"
          >
            {loading ? "Setting up..." : "Setup Secure Connection"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

// Protected Route Component
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading } = useAuth();

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600"></div>
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

// App Routes Component
function AppRoutes() {
  const { isAuthenticated, clientCertificate } = useAuth();

  // First check for certificate, then authentication
  if (!clientCertificate) {
    return <MTLSSetup />;
  }

  return (
    <Routes>
      <Route
        path="/login"
        element={
          isAuthenticated ? <Navigate to="/" replace /> : <LoginPage />
        }
      />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <Index />
          </ProtectedRoute>
        }
      />
      <Route
        path="/recent"
        element={
          <ProtectedRoute>
            <RecentPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/favorites"
        element={
          <ProtectedRoute>
            <Favorites />
          </ProtectedRoute>
        }
      />
      <Route
        path="/trash"
        element={
          <ProtectedRoute>
            <Trash />
          </ProtectedRoute>
        }
      />
      <Route
        path="/settings"
        element={
          <ProtectedRoute>
            <SettingsPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/profile"
        element={
          <ProtectedRoute>
            <UserProfile />
          </ProtectedRoute>
        }
      />
      <Route
        path="/user-management"
        element={
          <ProtectedRoute>
            <UserManagement />
          </ProtectedRoute>
        }
      />
      {/* ADD ALL CUSTOM ROUTES ABOVE THE CATCH-ALL "*" ROUTE */}
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}

const App = () => (
  <QueryClientProvider client={queryClient}>
    <AuthProvider>
      <TooltipProvider>
        <Toaster />
        <Sonner />
        <BrowserRouter>
          <AppRoutes />
        </BrowserRouter>
      </TooltipProvider>
    </AuthProvider>
  </QueryClientProvider>
);

export default App;
