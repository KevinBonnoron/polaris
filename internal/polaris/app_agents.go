package polaris

import "errors"

func (s *Service) GetProjectAgentDefaultModels(projectID string) (map[string]string, error) {
	if s.store == nil {
		return nil, errors.New("store not initialised")
	}
	return s.store.GetProjectAgentDefaultModels(projectID)
}

func (s *Service) SetProjectAgentDefaultModel(projectID, agentKind, modelID string) error {
	if s.store == nil {
		return errors.New("store not initialised")
	}
	return s.store.SetProjectAgentDefaultModel(projectID, agentKind, modelID)
}

func (s *Service) GetEffectiveAgentDefaultModel(projectID, agentKind string) (string, error) {
	if s.store == nil {
		return "", errors.New("store not initialised")
	}
	return s.store.GetEffectiveAgentDefaultModel(projectID, agentKind)
}
