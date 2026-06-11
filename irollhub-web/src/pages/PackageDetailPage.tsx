import { useParams } from 'react-router-dom';
import { PackageDetail } from '../components/package/PackageDetail';
import { api } from '../api/client';
import { useQuery } from '@tanstack/react-query';

export function PackageDetailPage() {
  const { org, pkg } = useParams<{ org: string; pkg: string }>();

  const { data, isLoading, error } = useQuery({
    queryKey: ['package', org, pkg],
    queryFn: () => api.getPackage(org || '', pkg || ''),
    enabled: !!org && !!pkg,
  });

  if (!org || !pkg) return <div>Package not found</div>;

  if (isLoading) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-secondary">Loading...</p></div>;
  if (error) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-red-500">Error loading package</p></div>;
  if (!data) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-secondary">Package not found</p></div>;

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
      <PackageDetail org={org} pkg={data} />
    </div>
  );
}
