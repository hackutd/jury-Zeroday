import { twMerge } from 'tailwind-merge';
import DragHamburger from './dnd/DragHamburger';

interface ProjectEntryProps {
    project: SortableJudgedProject;
    ranking: number;
    id: number;
    noDrag?: boolean;
    disabled?: boolean;
}

const ProjectEntry = (props: ProjectEntryProps) => {
    if (!props.project) {
        return null;
    }

    let rankColor = 'text-lightest';
    switch (props.ranking) {
        case 1:
            rankColor = 'text-gold';
            break;
        case 2:
            rankColor = 'text-light';
            break;
        case 3:
            rankColor = 'text-bronze';
            break;
        default:
            break;
    }

    return (
        <div className="flex items-center cursor-default">
            {props.ranking !== -1 && (
                <p className={twMerge('font-bold text-xl text-center w-8 shrink-0', rankColor)}>
                    {props.ranking}
                </p>
            )}
            <div className="m-1 pl-2 py-1 bg-background border-solid border-2 border-lightest rounded-md grow">
                <div className="flex flex-row">
                    <div className="grow">
                        <h3 className="text-lg leading-tight grow">
                            <a href={`/judge/project/${props.project.project_id}`}>
                                <b>Table {props.project.location}</b>
                                {': '}
                                {props.project.name}
                            </a>
                        </h3>
                        <p className="text-light text-xs line-clamp-1">{props.project.notes}</p>
                    </div>
                    {!props.noDrag ? (
                        <div className="text-right flex items-center justify-end">
                            <DragHamburger />
                        </div>
                    ) : (
                        <div className="w-2"></div>
                    )}
                </div>
            </div>
        </div>
    );
};

export default ProjectEntry;
