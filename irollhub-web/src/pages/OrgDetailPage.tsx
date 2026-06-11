import { useParams } from 'react-router-dom';
import { PackageList } from '../components/package/PackageList';
import { api } from '../api/client';
import { useQuery } from '@tanstack/react-query';

export function OrgDetailPage() {
  const { org } = useParams<{ org: string }>();

  const { data, isLoading, error } = useQuery({
    queryKey: ['org', org],
    queryFn: () => api.getOrg(org || ''),
    enabled: !!org,
  });

  if (!org) return <div>Organization not found</div>;

  if (isLoading) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-secondary">Loading...</p></div>;
  if (error) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-red-500">Error loading organization</p></div>;
  if (!data) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-secondary">Organization not found</p></div>;

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-primary mb-2">{data.name}</h1>
        <p className="text-secondary">
          Member since {new Date(data.created_at).toLocaleDateString()}
        </p>
      </div>
      <h2 className="text-2xl font-semibold text-primary mb-6">Packages</h2>
      <PackageList org={org} packages={data.packages || []} />
    </div>
  );
}
