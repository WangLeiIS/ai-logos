import { OrgList } from '../components/org/OrgList';
import { api } from '../api/client';
import { useQuery } from '@tanstack/react-query';

export function OrgsPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['orgs'],
    queryFn: () => api.listOrgs(100, 0),
  });

  const orgs = data?.data || [];

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
      <h1 className="text-3xl font-bold text-primary mb-6">Organizations</h1>
      {isLoading ? (
        <p className="text-secondary">Loading...</p>
      ) : error ? (
        <p className="text-red-500">Error loading organizations</p>
      ) : (
        <OrgList orgs={orgs} />
      )}
    </div>
  );
}
