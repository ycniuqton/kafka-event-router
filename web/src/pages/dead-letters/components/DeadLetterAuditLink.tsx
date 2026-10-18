import { useNavigate } from 'react-router-dom';
import { Can } from '../../../access';
import { Button } from '../../../components/ui';
import { Permission } from '../../../domain';
import { AuditPresetId, AuditPresets } from '../../audit/model/AuditPresets';

/** Header link to the audit log filtered to dead-letter actions (workspace admins only). */
export function DeadLetterAuditLink() {
  const navigate = useNavigate();
  return (
    <Can permission={Permission.AuditRead}>
      <Button variant="ghost" onClick={() => navigate(AuditPresets.path(AuditPresetId.DeadLetters))}>
        Audit trail →
      </Button>
    </Can>
  );
}
