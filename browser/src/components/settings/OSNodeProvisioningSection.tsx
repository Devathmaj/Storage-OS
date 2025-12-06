import React, { useState, useEffect } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Check, X, Copy, RefreshCw, ShieldCheck, ShieldAlert } from 'lucide-react';
import { toast } from 'sonner';
import { useAuth } from '@/contexts/AuthContext';

interface EnrollmentRequest {
  id: string;
  otp: string;
  system_info: string;
  parsed_system_info: {
    hostname: string;
    cpu: string;
    os_version: string;
    network_id: string;
  };
  requested_at: string;
  status: string;
}

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8081';

export const OSNodeProvisioningSection: React.FC = () => {
  const { token } = useAuth();
  const [otp, setOtp] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [pendingRequests, setPendingRequests] = useState<EnrollmentRequest[]>([]);
  const [loadingRequests, setLoadingRequests] = useState(false);

  // Generate OTP
  const handleGenerateOTP = async () => {
    setLoading(true);
    try {
      const response = await fetch(`${API_URL}/v1/provisioning/generate-otp`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        throw new Error('Failed to generate OTP');
      }

      const data = await response.json();
      setOtp(data.otp);
      toast.success('OTP generated successfully');
    } catch (error) {
      console.error('Error generating OTP:', error);
      toast.error('Failed to generate OTP');
    } finally {
      setLoading(false);
    }
  };

  // Copy OTP to clipboard
  const handleCopyOTP = () => {
    navigator.clipboard.writeText(otp);
    toast.success('OTP copied to clipboard');
  };

  // Load pending enrollment requests
  const loadPendingRequests = async () => {
    setLoadingRequests(true);
    try {
      const response = await fetch(`${API_URL}/v1/provisioning/enrollments/pending`, {
        headers: {
          'Authorization': `Bearer ${token}`,
        },
      });

      if (!response.ok) {
        throw new Error('Failed to load pending requests');
      }

      const data = await response.json();
      setPendingRequests(data.requests || []);
    } catch (error) {
      console.error('Error loading requests:', error);
      toast.error('Failed to load pending requests');
    } finally {
      setLoadingRequests(false);
    }
  };

  // Approve enrollment request
  const handleApprove = async (requestId: string) => {
    try {
      const response = await fetch(`${API_URL}/v1/provisioning/enrollments/${requestId}/approve`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        throw new Error('Failed to approve request');
      }

      toast.success('Enrollment request approved');
      loadPendingRequests();
    } catch (error) {
      console.error('Error approving request:', error);
      toast.error('Failed to approve request');
    }
  };

  // Reject enrollment request
  const handleReject = async (requestId: string) => {
    const reason = prompt('Enter rejection reason (optional):');
    
    try {
      const response = await fetch(`${API_URL}/v1/provisioning/enrollments/${requestId}/reject`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ reason: reason || 'Rejected by administrator' }),
      });

      if (!response.ok) {
        throw new Error('Failed to reject request');
      }

      toast.success('Enrollment request rejected');
      loadPendingRequests();
    } catch (error) {
      console.error('Error rejecting request:', error);
      toast.error('Failed to reject request');
    }
  };

  // Auto-refresh pending requests
  useEffect(() => {
    loadPendingRequests();
    const interval = setInterval(loadPendingRequests, 10000); // Refresh every 10 seconds
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="space-y-6">
      {/* OTP Generation Section */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="w-5 h-5" />
            OS Node Enrollment
          </CardTitle>
          <CardDescription>
            Generate one-time passwords (OTPs) for enrolling new OS nodes
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <Alert>
              <AlertDescription>
                <strong>Enrollment Process:</strong>
                <ol className="list-decimal list-inside mt-2 space-y-1">
                  <li>Generate an OTP from this dashboard</li>
                  <li>Enter the OTP on the OS node console during boot</li>
                  <li>Review and approve the enrollment request below</li>
                  <li>The OS node will receive its certificate and connect securely</li>
                </ol>
              </AlertDescription>
            </Alert>

            <div className="flex items-center gap-4">
              <Button 
                onClick={handleGenerateOTP} 
                disabled={loading}
                className="gap-2"
              >
                {loading ? 'Generating...' : 'Generate OTP'}
              </Button>

              {otp && (
                <div className="flex items-center gap-2 flex-1">
                  <div className="flex-1 p-3 border rounded-lg bg-muted font-mono text-2xl text-center tracking-wider">
                    {otp}
                  </div>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={handleCopyOTP}
                    title="Copy to clipboard"
                  >
                    <Copy className="w-4 h-4" />
                  </Button>
                </div>
              )}
            </div>

            {otp && (
              <Alert>
                <AlertDescription>
                  <strong>⏰ Valid for 5 minutes</strong> - Enter this OTP on the OS node console to begin enrollment.
                </AlertDescription>
              </Alert>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Pending Enrollment Requests */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Pending Enrollment Requests</CardTitle>
              <CardDescription>
                Review and approve or reject OS nodes awaiting enrollment
              </CardDescription>
            </div>
            <Button
              variant="outline"
              size="icon"
              onClick={loadPendingRequests}
              disabled={loadingRequests}
            >
              <RefreshCw className={`w-4 h-4 ${loadingRequests ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {pendingRequests.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              No pending enrollment requests
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Hostname</TableHead>
                  <TableHead>System Info</TableHead>
                  <TableHead>Requested</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pendingRequests.map((request) => (
                  <TableRow key={request.id}>
                    <TableCell className="font-medium">
                      {request.parsed_system_info?.hostname || 'Unknown'}
                    </TableCell>
                    <TableCell>
                      <div className="text-sm space-y-1">
                        <div><strong>CPU:</strong> {request.parsed_system_info?.cpu || 'Unknown'}</div>
                        <div><strong>OS:</strong> {request.parsed_system_info?.os_version || 'Unknown'}</div>
                        <div><strong>Network ID:</strong> {request.parsed_system_info?.network_id || 'Unknown'}</div>
                      </div>
                    </TableCell>
                    <TableCell>
                      {new Date(request.requested_at).toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="default"
                          onClick={() => handleApprove(request.id)}
                          className="gap-1"
                        >
                          <Check className="w-4 h-4" />
                          Approve
                        </Button>
                        <Button
                          size="sm"
                          variant="destructive"
                          onClick={() => handleReject(request.id)}
                          className="gap-1"
                        >
                          <X className="w-4 h-4" />
                          Reject
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
};
